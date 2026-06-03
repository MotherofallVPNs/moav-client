package main

import (
	"context"
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
	"github.com/ibeezhan/moav-client/proxy-core/logbus"
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
	"github.com/ibeezhan/moav-client/proxy-core/prober"
	"github.com/ibeezhan/moav-client/proxy-core/proxy"
	"github.com/ibeezhan/moav-client/proxy-core/sidecars"
	"github.com/ibeezhan/moav-client/proxy-core/singbox"
	"github.com/ibeezhan/moav-client/proxy-core/state"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
	"github.com/ibeezhan/moav-client/proxy-core/xray"
)

const statePath = "data/state.json"

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
			ep.Source = source
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
			tmp := outPath + ".tmp"
			if err := os.WriteFile(tmp, jsonBytes, 0o644); err != nil {
				log.Printf("singbox: write %s: %v", tmp, err)
			} else if err := os.Rename(tmp, outPath); err != nil {
				log.Printf("singbox: rename %s -> %s: %v", tmp, outPath, err)
			} else {
				log.Printf("singbox: wrote %d-endpoint config to %s (dial via %s)", len(endpoints), outPath, cfg.Singbox.DialHost)
				endpoints = updatedEps
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
			tmp := outPath + ".tmp"
			if err := os.WriteFile(tmp, jsonBytes, 0o644); err != nil {
				log.Printf("xray: write %s: %v", tmp, err)
			} else if err := os.Rename(tmp, outPath); err != nil {
				log.Printf("xray: rename %s -> %s: %v", tmp, outPath, err)
			} else {
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
					if p2, ok := probed[current[i].ID]; ok {
						current[i].Status = p2.Status
						current[i].LatencyMs = p2.LatencyMs
					}
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
		engineRules = append(engineRules, plugins.Rule{
			Match:      m,
			Action:     action,
			ActionName: plugins.DecisionName(action),
			Enabled:    true,
		})
	}
	eng := plugins.NewEngine(engineRules)
	tb := &plugins.TorrentBlocker{Enabled: cfg.Plugins.TorrentBlock}

	proxyServer := proxy.NewServer(cfg.Proxy.SOCKS5Port, cfg.Proxy.HTTPPort, b, eng, tb)
	// SOCKS5 auth: prefer .env vars (set by the dashboard's Network exposure
	// tab) over config.yaml; this way users who flip exposure→LAN in the UI
	// don't have to re-edit YAML to get auth applied.
	socksUser, socksPass := cfg.Proxy.Auth.Username, cfg.Proxy.Auth.Password
	if v := os.Getenv("SOCKS5_USERNAME"); v != "" {
		socksUser = v
	}
	if v := os.Getenv("SOCKS5_PASSWORD"); v != "" {
		socksPass = v
	}
	if socksUser != "" && socksPass != "" {
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
