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

// maxDialAttempts caps how many endpoints we'll try before falling back to a
// direct dial. Keeps a single bad request from cycling through every peer.
const maxDialAttempts = 4

// DialContext dials the destination through the selected endpoint, with
// automatic failover: on dial error, the failed endpoint is marked errored
// and the next-best endpoint is picked. Direct dial is the final fallback.
func (b *Balancer) DialContext(network, addr string) (net.Conn, error) {
	tried := make(map[string]struct{}, maxDialAttempts)

	for attempt := 0; attempt < maxDialAttempts; attempt++ {
		ep, err := b.pickExcluding(tried)
		if err != nil {
			break
		}
		conn, dialErr := dialThrough(ep, network, addr)
		if dialErr == nil {
			if attempt > 0 {
				log.Printf("balancer: dial %s via %s succeeded after %d failover(s)", addr, ep.ID, attempt)
			}
			return conn, nil
		}
		b.markError(ep.ID)
		tried[ep.ID] = struct{}{}
		log.Printf("balancer: dial through %s failed (%v); trying next endpoint", ep.ID, dialErr)
	}

	log.Printf("balancer: all candidates failed, dialing %s directly", addr)
	return net.Dial(network, addr)
}

// pickExcluding returns the best endpoint not in the excluded set.
func (b *Balancer) pickExcluding(excluded map[string]struct{}) (*subscription.Endpoint, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var live []subscription.Endpoint
	for _, ep := range b.endpoints {
		if !ep.Enabled || ep.Status != "ok" {
			continue
		}
		if _, skip := excluded[ep.ID]; skip {
			continue
		}
		live = append(live, ep)
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
//
// If Config["socks5_addr"] is set, we dial through that local SOCKS5 port
// (this is how sing-box-backed endpoints and sidecars expose themselves).
// Otherwise we fall back to legacy behaviour: SOCKS5 to ep.Address for
// protocols that can be naively tunneled that way, error for the rest.
func dialThrough(ep *subscription.Endpoint, network, addr string) (net.Conn, error) {
	if socksAddr := ep.Config["socks5_addr"]; socksAddr != "" {
		return dialSOCKS5(socksAddr, network, addr)
	}

	switch ep.Protocol {
	case "hysteria2", "wireguard":
		return nil, fmt.Errorf("protocol %s requires sing-box (no socks5_addr set)", ep.Protocol)

	case "sidecar":
		return dialSOCKS5("127.0.0.1:1080", network, addr)

	default:
		// Legacy fallback: treat ep.Address as SOCKS5 endpoint.
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
