package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ibeezhan/moav-client/proxy-core/api"
	"github.com/ibeezhan/moav-client/proxy-core/balancer"
	"github.com/ibeezhan/moav-client/proxy-core/config"
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
	"github.com/ibeezhan/moav-client/proxy-core/prober"
	"github.com/ibeezhan/moav-client/proxy-core/proxy"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

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

	// Parse subscription from file or URL.
	var endpoints []subscription.Endpoint
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
				endpoints = eps
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

			// Start background probing loop.
			ch := p.Run(ctx, updated)
			for eps := range ch {
				b.SetEndpoints(eps)
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
