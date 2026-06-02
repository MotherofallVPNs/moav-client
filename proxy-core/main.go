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
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
	"github.com/ibeezhan/moav-client/proxy-core/prober"
	"github.com/ibeezhan/moav-client/proxy-core/proxy"
	"github.com/ibeezhan/moav-client/proxy-core/sidecars"
	"github.com/ibeezhan/moav-client/proxy-core/singbox"
	"github.com/ibeezhan/moav-client/proxy-core/state"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
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

	// Parse subscription from file and/or URL; merge and deduplicate by RawURI.
	seen := make(map[string]struct{})
	var endpoints []subscription.Endpoint

	addEndpoints := func(eps []subscription.Endpoint) {
		for _, ep := range eps {
			if _, dup := seen[ep.RawURI]; dup {
				continue
			}
			seen[ep.RawURI] = struct{}{}
			// Restore saved latency/status if available.
			if saved, ok := stateByURI[ep.RawURI]; ok {
				ep.LatencyMs = saved.LatencyMs
				ep.Status = saved.Status
			}
			endpoints = append(endpoints, ep)
		}
	}

	if cfg.Subscription.File != "" {
		raw, readErr := os.ReadFile(cfg.Subscription.File)
		if readErr != nil {
			log.Printf("subscription: could not read %s: %v", cfg.Subscription.File, readErr)
		} else {
			eps, parseErr := subscription.ParseSubscription(string(raw))
			if parseErr != nil {
				log.Printf("subscription: parse error: %v", parseErr)
			} else {
				log.Printf("subscription: loaded %d endpoints from %s", len(eps), cfg.Subscription.File)
				addEndpoints(eps)
			}
		}
	}

	if cfg.Subscription.URL != "" {
		eps, fetchErr := subscription.FetchSubscription(cfg.Subscription.URL, 30*time.Second)
		if fetchErr != nil {
			log.Printf("subscription: fetch error from %s: %v", cfg.Subscription.URL, fetchErr)
		} else {
			log.Printf("subscription: fetched %d endpoints from %s", len(eps), cfg.Subscription.URL)
			addEndpoints(eps)
		}
	}

	// Wireguard / AmneziaWG .conf sidecars.
	for _, wgPath := range cfg.Subscription.WireGuardFiles {
		raw, readErr := os.ReadFile(wgPath)
		if readErr != nil {
			log.Printf("subscription: could not read %s: %v", wgPath, readErr)
			continue
		}
		nameHint := strings.TrimSuffix(filepath.Base(wgPath), filepath.Ext(wgPath))
		ep, parseErr := subscription.ParseWireGuardConf(string(raw), nameHint)
		if parseErr != nil {
			log.Printf("subscription: wg conf %s parse error: %v", wgPath, parseErr)
			continue
		}
		// AmneziaWG can't be dialed by sing-box (no outbound for the
		// obfuscation params). If the user enabled the amneziawg sidecar,
		// that's the real dial path — skip this duplicate to keep the
		// pool clean. The .conf was still consumed by configgen.
		if ep.Protocol == "amneziawg" && cfg.Sidecars.AmneziaWG.Enabled {
			log.Printf("subscription: %s endpoint %q from %s superseded by sidecar", ep.Protocol, ep.Name, wgPath)
			continue
		}
		log.Printf("subscription: loaded %s endpoint %q from %s", ep.Protocol, ep.Name, wgPath)
		addEndpoints([]subscription.Endpoint{ep})
	}

	// Add sidecar endpoints + write per-sidecar config files.
	sm := &sidecars.SidecarManager{Config: cfg.Sidecars}
	if err := sm.GenerateConfigs("data/sidecar-configs"); err != nil {
		log.Printf("sidecars: configgen: %v", err)
	}
	addEndpoints(sm.EnabledEndpoints())

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

			// Start background probing loop.
			ch := p.Run(ctx, updated)
			for eps := range ch {
				b.SetEndpoints(eps)
				// Persist after every background probe cycle.
				s2 := &state.State{LastProbeAt: time.Now(), Endpoints: eps}
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
		engineRules = append(engineRules, plugins.Rule{Match: m, Action: action})
	}
	eng := plugins.NewEngine(engineRules)
	tb := &plugins.TorrentBlocker{Enabled: cfg.Plugins.TorrentBlock}

	proxyServer := proxy.NewServer(cfg.Proxy.SOCKS5Port, cfg.Proxy.HTTPPort, b, eng, tb)
	if cfg.Proxy.Auth.Username != "" && cfg.Proxy.Auth.Password != "" {
		proxyServer = proxyServer.WithAuth(cfg.Proxy.Auth.Username, cfg.Proxy.Auth.Password)
	}
	apiServer := api.New(cfg.Proxy.APIPort, b)

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
