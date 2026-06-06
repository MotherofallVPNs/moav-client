package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ibeezhan/moav-client/proxy-core/api"
	"github.com/ibeezhan/moav-client/proxy-core/balancer"
	"github.com/ibeezhan/moav-client/proxy-core/cmd"
	"github.com/ibeezhan/moav-client/proxy-core/config"
	"github.com/ibeezhan/moav-client/proxy-core/dockerctl"
	"github.com/ibeezhan/moav-client/proxy-core/logbus"
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
	"github.com/ibeezhan/moav-client/proxy-core/prober"
	"github.com/ibeezhan/moav-client/proxy-core/proxy"
	"github.com/ibeezhan/moav-client/proxy-core/sidecars"
	"github.com/ibeezhan/moav-client/proxy-core/singbox"
	"github.com/ibeezhan/moav-client/proxy-core/snispoof"
	"github.com/ibeezhan/moav-client/proxy-core/state"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
	"github.com/ibeezhan/moav-client/proxy-core/xray"
)

const statePath = "data/state.json"

// endpointEligibleForSNISpoof returns true for endpoints that actually do
// a TCP+TLS handshake on the wire — the only case where injecting a fake
// ClientHello is meaningful. Reality, Shadowsocks (own crypto), UDP/QUIC
// protocols, and sidecar endpoints are excluded.
func endpointEligibleForSNISpoof(ep subscription.Endpoint) bool {
	if ep.Protocol == "sidecar" {
		return false
	}
	if ep.Config["security"] == "reality" {
		return false
	}
	switch ep.Protocol {
	case "vless", "trojan", "vmess":
		// Only when TLS is actually in play.
		return ep.Config["security"] == "tls"
	}
	return false
}

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config.yaml")

	// Parse global flags first (before subcommand dispatch).
	// We must handle the case where the first arg is a subcommand (not a flag).
	if len(os.Args) >= 2 && len(os.Args[1]) > 0 && os.Args[1][0] != '-' {
		// Subcommand present — parse global flags from args after the subcommand.
		flag.CommandLine.Parse(os.Args[2:]) //nolint:errcheck
	} else {
		flag.Parse()
	}

	subcmd := cmd.ParseAndRun(cfgPath)
	if subcmd != "serve" {
		return
	}

	// Capture every log.Printf into the bus so the dashboard's Debug tab
	// can show a live tail. Keep stderr passthrough so `docker logs` works.
	// Per-level ring of 800 so the user sees plenty of warn/error history
	// even under heavy INFO traffic.
	logbus.Default = logbus.New(800)
	logbus.CaptureStdLog(logbus.Default, "proxy-core", os.Stderr.Write)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Printf("moav-client starting — SOCKS5 :%d  HTTP :%d  API :%d",
		cfg.Proxy.SOCKS5Port, cfg.Proxy.HTTPPort, cfg.Proxy.APIPort)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	strategy := balancer.Strategy(cfg.LoadBalancing.Strategy)
	if strategy == "" {
		strategy = balancer.StrategyLatency
	}
	b := balancer.New(strategy)

	// Load persisted state (restores LatencyMs/Status from last run).
	savedState, stateErr := state.Load(statePath)
	if stateErr != nil {
		log.Printf("state: load error (starting fresh): %v", stateErr)
		savedState = &state.State{}
	}
	stateByURI := make(map[string]subscription.Endpoint, len(savedState.Endpoints))
	for _, ep := range savedState.Endpoints {
		stateByURI[ep.RawURI] = ep
	}

	// Parse every effective source (legacy single-source fields + sources[]).
	// Each source contributes endpoints tagged with .Source = source.Name.
	seen := make(map[string]struct{})
	var endpoints []subscription.Endpoint

	addEndpoints := func(eps []subscription.Endpoint, source string) {
		for _, ep := range eps {
			if _, dup := seen[ep.RawURI]; dup {
				continue
			}
			seen[ep.RawURI] = struct{}{}
			// Keep a pre-set Source (sidecars tagged with their origin bundle);
			// otherwise attribute to the group/source being added.
			if ep.Source == "" {
				ep.Source = source
			}
			// Namespace the ID by source so endpoints from two configs
			// pointing at the same moav server (same protocol:host:port)
			// don't collide. Sidecar IDs are already "sidecar:<kind>"
			// and unique by construction — skip those. "default" is the
			// legacy/un-named source so we leave it bare for backcompat
			// with persisted state.
			if source != "" && source != "default" && source != "sidecars" {
				ep.ID = source + "/" + ep.ID
			}
			// Restore saved latency/status if available.
			if saved, ok := stateByURI[ep.RawURI]; ok {
				ep.LatencyMs = saved.LatencyMs
				ep.Status = saved.Status
				// Restore user overrides (Enabled / Priority) so toggles
				// made from the dashboard survive restart.
				ep.Enabled = saved.Enabled
				if saved.Priority != 0 {
					ep.Priority = saved.Priority
				}
			}
			endpoints = append(endpoints, ep)
		}
	}

	for _, src := range cfg.Subscription.EffectiveSources() {
		if src.File != "" {
			raw, readErr := os.ReadFile(src.File)
			if readErr != nil {
				log.Printf("subscription[%s]: could not read %s: %v", src.Name, src.File, readErr)
			} else {
				eps, parseErr := subscription.ParseSubscription(string(raw))
				if parseErr != nil {
					log.Printf("subscription[%s]: parse error: %v", src.Name, parseErr)
				} else {
					log.Printf("subscription[%s]: loaded %d endpoints from %s", src.Name, len(eps), src.File)
					addEndpoints(eps, src.Name)
				}
			}
		}

		if src.URL != "" {
			eps, fetchErr := subscription.FetchSubscription(src.URL, 30*time.Second)
			if fetchErr != nil {
				log.Printf("subscription[%s]: fetch error from %s: %v", src.Name, src.URL, fetchErr)
			} else {
				log.Printf("subscription[%s]: fetched %d endpoints from %s", src.Name, len(eps), src.URL)
				addEndpoints(eps, src.Name)
			}
		}

		// Wireguard / AmneziaWG .conf files for this source.
		for _, wgPath := range src.WireGuardFiles {
			raw, readErr := os.ReadFile(wgPath)
			if readErr != nil {
				log.Printf("subscription[%s]: could not read %s: %v", src.Name, wgPath, readErr)
				continue
			}
			nameHint := strings.TrimSuffix(filepath.Base(wgPath), filepath.Ext(wgPath))
			if src.Name != "" && src.Name != "default" {
				nameHint = src.Name + "/" + nameHint
			}
			ep, parseErr := subscription.ParseWireGuardConf(string(raw), nameHint)
			if parseErr != nil {
				log.Printf("subscription[%s]: wg conf %s parse error: %v", src.Name, wgPath, parseErr)
				continue
			}
			// AmneziaWG can't be dialed by sing-box (no outbound for the
			// obfuscation params). If the user enabled the amneziawg sidecar,
			// that's the real dial path — skip this duplicate to keep the
			// pool clean.
			if ep.Protocol == "amneziawg" && cfg.Sidecars.AmneziaWG.Enabled {
				log.Printf("subscription[%s]: %s endpoint %q from %s superseded by sidecar", src.Name, ep.Protocol, ep.Name, wgPath)
				continue
			}
			log.Printf("subscription[%s]: loaded %s endpoint %q from %s", src.Name, ep.Protocol, ep.Name, wgPath)
			addEndpoints([]subscription.Endpoint{ep}, src.Name)
		}
	}

	// Add sidecar endpoints + write per-sidecar config files.
	sm := &sidecars.SidecarManager{Config: cfg.Sidecars}
	if err := sm.GenerateConfigs("data/sidecar-configs"); err != nil {
		log.Printf("sidecars: configgen: %v", err)
	}
	addEndpoints(sm.EnabledEndpoints(), "sidecars")

	// Apply SNI-spoof config first (before sing-box / xray). For every
	// endpoint with fake_sni set, allocate a sniproof port and rewrite
	// Config["spoof_via"] so the downstream generator points sing-box /
	// xray at the spoofer rather than the real upstream. Reality is
	// auto-excluded — its handshake auth breaks under a faked CH.
	if cfg.SNISpoof.Enabled && len(endpoints) > 0 {
		// Promote DefaultFakeSNI / DefaultUTLS onto every endpoint that's
		// actually doing a TCP+TLS handshake — i.e. where slipping a fake
		// CH onto the wire is meaningful. Excludes:
		//   - Reality (handshake auth doesn't survive a faked CH)
		//   - Hysteria2 + WireGuard + AmneziaWG + TUIC (UDP/QUIC, not TLS)
		//   - Shadowsocks (own crypto, no TLS hello on the wire)
		//   - sidecar endpoints (the sidecar itself is the dial target)
		if cfg.SNISpoof.DefaultFakeSNI != "" {
			for i := range endpoints {
				if endpoints[i].Config == nil {
					endpoints[i].Config = map[string]string{}
				}
				if !endpointEligibleForSNISpoof(endpoints[i]) {
					continue
				}
				if endpoints[i].Config["fake_sni"] == "" {
					endpoints[i].Config["fake_sni"] = cfg.SNISpoof.DefaultFakeSNI
				}
				if endpoints[i].Config["utls"] == "" && cfg.SNISpoof.DefaultUTLS != "" {
					endpoints[i].Config["utls"] = cfg.SNISpoof.DefaultUTLS
				}
			}
		}
		ssCfg := snispoof.Config{
			ListenHost: cfg.SNISpoof.ListenHost,
			DialHost:   cfg.SNISpoof.DialHost,
			BasePort:   cfg.SNISpoof.BasePort,
		}
		jsonBytes, updatedEps, err := snispoof.Generate(endpoints, ssCfg)
		if err != nil {
			log.Printf("snispoof: generate error: %v", err)
		} else if jsonBytes == nil {
			log.Printf("snispoof: no endpoints carry fake_sni; sidecar will idle")
		} else {
			outPath := cfg.SNISpoof.OutputPath
			if outPath == "" {
				outPath = "data/sni-spoof.json"
			}
			tmp := outPath + ".tmp"
			if err := os.WriteFile(tmp, jsonBytes, 0o644); err != nil {
				log.Printf("snispoof: write %s: %v", tmp, err)
			} else if err := os.Rename(tmp, outPath); err != nil {
				log.Printf("snispoof: rename %s -> %s: %v", tmp, outPath, err)
			} else {
				count := 0
				for _, ep := range updatedEps {
					if ep.Config["spoof_via"] != "" {
						count++
					}
				}
				log.Printf("snispoof: wrote %d-mapping config to %s (dial via %s)", count, outPath, cfg.SNISpoof.DialHost)
				endpoints = updatedEps
			}
		}
	}

	// Generate sing-box config and rewrite endpoints to dial through local
	// sing-box SOCKS5 ports for real protocol cryptography.
	if cfg.Singbox.Enabled && len(endpoints) > 0 {
		sbCfg := singbox.Config{
			ListenHost: cfg.Singbox.ListenHost,
			DialHost:   cfg.Singbox.DialHost,
			BasePort:   cfg.Singbox.BasePort,
		}
		jsonBytes, updatedEps, err := singbox.Generate(endpoints, sbCfg)
		if err != nil {
			log.Printf("singbox: generate error: %v", err)
		} else {
			outPath := cfg.Singbox.OutputPath
			if outPath == "" {
				outPath = "data/singbox.json"
			}
			changed, werr := writeConfigIfChanged(outPath, jsonBytes)
			if werr != nil {
				log.Printf("singbox: %v", werr)
			} else {
				log.Printf("singbox: wrote %d-endpoint config to %s (dial via %s, changed=%v)", len(endpoints), outPath, cfg.Singbox.DialHost, changed)
				endpoints = updatedEps
				// Always cycle sing-box on proxy-core startup: even when
				// the on-disk file is unchanged, sing-box may have been
				// launched against an older revision of it before this
				// proxy-core run. Cheap (~2 s) and idempotent.
				maybeRestartContainer("singbox")
			}
		}
	}

	// Then Xray for the leftovers — endpoints whose transport sing-box doesn't
	// speak (xhttp / splithttp / raw). Endpoints already pinned to sing-box
	// stay untouched because xray.Generate's HandlesEndpoint filter is exclusive.
	if cfg.Xray.Enabled && len(endpoints) > 0 {
		xCfg := xray.Config{
			ListenHost: cfg.Xray.ListenHost,
			DialHost:   cfg.Xray.DialHost,
			BasePort:   cfg.Xray.BasePort,
		}
		jsonBytes, updatedEps, err := xray.Generate(endpoints, xCfg)
		if err != nil {
			log.Printf("xray: generate error: %v", err)
		} else if jsonBytes == nil {
			log.Printf("xray: no Xray-only endpoints found; not writing %s", cfg.Xray.OutputPath)
		} else {
			outPath := cfg.Xray.OutputPath
			if outPath == "" {
				outPath = "data/xray.json"
			}
			changed, werr := writeConfigIfChanged(outPath, jsonBytes)
			if werr != nil {
				log.Printf("xray: %v", werr)
			} else {
				_ = changed
				maybeRestartContainer("xray")
				count := 0
				for _, ep := range updatedEps {
					if strings.HasPrefix(ep.Config["socks5_addr"], cfg.Xray.DialHost+":") {
						count++
					}
				}
				log.Printf("xray: wrote %d-endpoint config to %s (dial via %s)", count, outPath, cfg.Xray.DialHost)
				endpoints = updatedEps
			}
		}
	}

	b.SetEndpoints(endpoints)

	// Probe endpoints on start if configured.
	if cfg.LoadBalancing.ProbeOnStart && len(endpoints) > 0 {
		p := prober.New()
		go func() {
			updated := p.ProbeAll(endpoints)
			b.SetEndpoints(updated)
			log.Printf("initial probe complete: %d endpoints updated", len(updated))

			// Persist state after first probe.
			s := &state.State{LastProbeAt: time.Now(), Endpoints: updated}
			if err := s.Save(statePath); err != nil {
				log.Printf("state: save error: %v", err)
			}

			// Start background probing loop. We pass a fetcher that pulls
			// from the LIVE balancer pool so user PATCHes (enable/disable,
			// priority) from the dashboard survive each cycle instead of
			// being clobbered by the stale slice we started with.
			ch := p.Run(ctx, b.Endpoints)
			prevStatus := map[string]string{}
			for eps := range ch {
				// Merge probe results into the live pool: copy Status /
				// LatencyMs from probed copies onto current entries, but
				// keep current Enabled / Priority intact.
				current := b.Endpoints()
				probed := make(map[string]subscription.Endpoint, len(eps))
				for _, ep := range eps {
					probed[ep.ID] = ep
				}
				for i := range current {
					p2, ok := probed[current[i].ID]
					if !ok {
						continue
					}
					// Emit a status-transition event at WARN level so the
					// Debug tab gets a clear "this endpoint just flipped"
					// signal — not just the periodic cycle summary.
					prev := prevStatus[current[i].ID]
					if prev != "" && prev != p2.Status {
						name := current[i].Name
						if name == "" {
							name = current[i].ID
						}
						if p2.Status == "ok" {
							log.Printf("endpoint %s recovered: %s → ok", name, prev)
						} else {
							log.Printf("endpoint %s went unhealthy: %s → %s", name, prev, p2.Status)
						}
					}
					prevStatus[current[i].ID] = p2.Status
					current[i].Status = p2.Status
					current[i].LatencyMs = p2.LatencyMs
				}
				b.SetEndpoints(current)
				// Persist after every background probe cycle.
				s2 := &state.State{LastProbeAt: time.Now(), Endpoints: current}
				if err := s2.Save(statePath); err != nil {
					log.Printf("state: save error: %v", err)
				}
			}
		}()
	}

	// Build plugin engine from config.
	var engineRules []plugins.Rule
	for _, rc := range cfg.Plugins.RoutingRules {
		m := plugins.MatchExpr{Type: rc.Match.Type, Value: rc.Match.Value}
		var action plugins.Decision
		switch rc.Action {
		case "direct":
			action = plugins.DecisionDirect
		case "block":
			action = plugins.DecisionBlock
		default:
			action = plugins.DecisionProxy
		}
		// Absent `enabled` (older / hand-written configs) defaults to true.
		enabled := rc.Enabled == nil || *rc.Enabled
		engineRules = append(engineRules, plugins.Rule{
			Match:      m,
			Action:     action,
			ActionName: plugins.DecisionName(action),
			Enabled:    enabled,
			Note:       rc.Note,
		})
	}
	eng := plugins.NewEngine(engineRules)
	eng.SetBlockDirect(cfg.Plugins.BlockDirect)
	b.SetBlockDirect(cfg.Plugins.BlockDirect)
	tb := &plugins.TorrentBlocker{Enabled: cfg.Plugins.TorrentBlock}

	proxyServer := proxy.NewServer(cfg.Proxy.SOCKS5Port, cfg.Proxy.HTTPPort, b, eng, tb)
	// SOCKS5 auth precedence: .env FILE (live, written by the dashboard) >
	// process env (baked into the container at creation by env_file) >
	// config.yaml. Reading the file means a plain `restart` re-applies an
	// auth change from the dashboard — without it, env_file values only
	// refresh on a full container recreate.
	envFile := readDotEnv(".env")
	socksUser, socksPass := cfg.Proxy.Auth.Username, cfg.Proxy.Auth.Password
	for _, v := range []string{os.Getenv("SOCKS5_USERNAME"), envFile["SOCKS5_USERNAME"]} {
		if v != "" {
			socksUser = v
		}
	}
	for _, v := range []string{os.Getenv("SOCKS5_PASSWORD"), envFile["SOCKS5_PASSWORD"]} {
		if v != "" {
			socksPass = v
		}
	}
	// A password is enough to turn auth on. If no username was given, default to
	// "moav" so clients have a predictable username. Requiring both would
	// silently leave the proxy open when someone sets only a password.
	if socksPass != "" {
		if socksUser == "" {
			socksUser = "moav"
		}
		proxyServer = proxyServer.WithAuth(socksUser, socksPass)
	}
	apiServer := api.New(cfg.Proxy.APIPort, *cfgPath, statePath, b, eng)

	errCh := make(chan error, 3)

	go func() { errCh <- proxyServer.ListenAndServeSOCKS5(ctx) }()
	go func() { errCh <- proxyServer.ListenAndServeHTTP(ctx) }()
	go func() { errCh <- apiServer.ListenAndServe(ctx) }()

	select {
	case <-ctx.Done():
		log.Println("shutting down")
	case err := <-errCh:
		if err != nil {
			log.Fatalf("fatal: %v", err)
		}
	}
}

// readDotEnv parses a KEY=VALUE .env file into a map. Best-effort: returns an
// empty map if the file is missing. Lets a dashboard-written auth change apply
// on a plain restart (the container's env_file only refreshes on recreate).
func readDotEnv(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}

// writeConfigIfChanged atomically writes content to path via .tmp + rename.
// Returns (changed, err) where `changed` is true iff the new bytes differ
// from what was already on disk — callers use this to decide whether to
// kick a downstream container (sing-box / xray) into restart so it reloads.
func writeConfigIfChanged(path string, content []byte) (bool, error) {
	newHash := sha256.Sum256(content)
	var oldHash [32]byte
	if old, err := os.ReadFile(path); err == nil {
		oldHash = sha256.Sum256(old)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return false, err
	}
	return oldHash != newHash, nil
}

// maybeRestartContainer issues a docker restart against the named
// docker-compose service in a background goroutine — no-op if the docker
// socket isn't mounted (dev / standalone use). Used to hot-cycle sing-box
// / xray after their config files are regenerated, otherwise they keep
// serving the listener set they loaded at startup and any new endpoints
// added during this proxy-core run dial against ports nobody is bound to.
func maybeRestartContainer(service string) {
	if !dockerctl.Available() {
		log.Printf("%s: config changed but docker socket unmounted — restart %s manually to pick up new endpoints", service, service)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		c := dockerctl.New()
		id, err := c.FindContainerByService(ctx, service)
		if err != nil || id == "" {
			log.Printf("%s: couldn't find container to restart: %v", service, err)
			return
		}
		if err := c.Restart(ctx, id); err != nil {
			log.Printf("%s: restart failed: %v", service, err)
			return
		}
		log.Printf("%s: restarted to reload regenerated config", service)
	}()
}
