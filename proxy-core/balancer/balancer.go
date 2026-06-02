// Package balancer selects the best upstream endpoint for each connection.
package balancer

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sort"
	"sync"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
	"golang.org/x/net/proxy"
)

// Strategy describes how the balancer picks endpoints.
type Strategy string

const (
	StrategyLatency  Strategy = "latency"
	StrategyPriority Strategy = "priority"
	StrategyWeighted Strategy = "weighted"
)

// ErrNoEndpoints is returned when no live endpoint is available.
var ErrNoEndpoints = errors.New("balancer: no healthy endpoints available")

// Balancer holds the endpoint pool and selects upstreams.
type Balancer struct {
	mu        sync.RWMutex
	endpoints []subscription.Endpoint
	strategy  Strategy
}

// New creates a Balancer with the given strategy.
func New(strategy Strategy) *Balancer {
	return &Balancer{strategy: strategy}
}

// SetEndpoints atomically replaces the endpoint pool.
func (b *Balancer) SetEndpoints(eps []subscription.Endpoint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.endpoints = eps
}

// Endpoints returns a snapshot of the current endpoint pool.
func (b *Balancer) Endpoints() []subscription.Endpoint {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := make([]subscription.Endpoint, len(b.endpoints))
	copy(cp, b.endpoints)
	return cp
}

// Pick returns the best available endpoint according to the configured strategy.
// Only endpoints with Enabled == true and Status == "ok" are considered.
func (b *Balancer) Pick() (*subscription.Endpoint, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var live []subscription.Endpoint
	for _, ep := range b.endpoints {
		if ep.Enabled && ep.Status == "ok" {
			live = append(live, ep)
		}
	}
	if len(live) == 0 {
		return nil, ErrNoEndpoints
	}

	var chosen subscription.Endpoint
	switch b.strategy {
	case StrategyLatency:
		sort.Slice(live, func(i, j int) bool {
			return live[i].LatencyMs < live[j].LatencyMs
		})
		chosen = live[0]

	case StrategyPriority:
		sort.Slice(live, func(i, j int) bool {
			return live[i].Priority < live[j].Priority
		})
		chosen = live[0]

	case StrategyWeighted:
		chosen = weightedRandom(live)

	default:
		chosen = live[0]
	}

	ep := chosen
	return &ep, nil
}

// DialContext dials the destination through the selected endpoint.
// For protocols not directly dialable (hysteria2, wireguard), the endpoint is
// skipped and an error is returned.
func (b *Balancer) DialContext(network, addr string) (net.Conn, error) {
	ep, err := b.Pick()
	if err != nil {
		// No healthy proxy — fall back to direct dial.
		log.Printf("balancer: no healthy endpoint, dialing %s directly", addr)
		return net.Dial(network, addr)
	}

	conn, dialErr := dialThrough(ep, network, addr)
	if dialErr != nil {
		// Mark endpoint as error and try direct.
		b.markError(ep.ID)
		log.Printf("balancer: dial through %s failed (%v), falling back direct", ep.ID, dialErr)
		return net.Dial(network, addr)
	}
	return conn, nil
}

// markError marks the endpoint with the given ID as errored.
func (b *Balancer) markError(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.endpoints {
		if b.endpoints[i].ID == id {
			b.endpoints[i].Status = "error"
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// dialThrough creates a connection to addr through the given endpoint.
// Protocols that cannot be dialed directly return an error so the caller
// can fall back to direct or try another endpoint.
func dialThrough(ep *subscription.Endpoint, network, addr string) (net.Conn, error) {
	switch ep.Protocol {
	case "hysteria2", "wireguard":
		return nil, fmt.Errorf("protocol %s not yet directly dialable, skipping", ep.Protocol)

	case "sidecar":
		// Sidecar exposes a local SOCKS5 port.
		socksAddr := ep.Config["socks5_addr"]
		if socksAddr == "" {
			socksAddr = "127.0.0.1:1080"
		}
		return dialSOCKS5(socksAddr, network, addr)

	default:
		// vless, vmess, trojan, ss, tuic: connect via SOCKS5 upstream.
		// These protocols require a client-side implementation to fully tunnel;
		// for now we connect over SOCKS5 assuming the endpoint is a SOCKS5
		// server (e.g. xray/sing-box sidecar configured on the same host).
		return dialSOCKS5(ep.Address, network, addr)
	}
}

// dialSOCKS5 connects to addr through a SOCKS5 proxy at proxyAddr.
func dialSOCKS5(proxyAddr, network, addr string) (net.Conn, error) {
	dialer, err := proxy.SOCKS5(network, proxyAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer for %s: %w", proxyAddr, err)
	}
	return dialer.Dial(network, addr)
}

// weightedRandom picks an endpoint by weight (stored in Config["upload_weight"]).
func weightedRandom(eps []subscription.Endpoint) subscription.Endpoint {
	total := 0
	weights := make([]int, len(eps))
	for i, ep := range eps {
		w := 1
		if ep.Priority > 0 {
			w = ep.Priority
		}
		weights[i] = w
		total += w
	}
	r := rand.Intn(total)
	for i, w := range weights {
		r -= w
		if r < 0 {
			return eps[i]
		}
	}
	return eps[len(eps)-1]
}
