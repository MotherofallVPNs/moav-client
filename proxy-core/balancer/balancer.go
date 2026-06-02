// Package balancer selects the best upstream endpoint for each connection.
package balancer

import (
	"net"
	"sync"
	"time"
)

// Strategy describes how the balancer picks endpoints.
type Strategy string

const (
	StrategyLatency  Strategy = "latency"
	StrategyPriority Strategy = "priority"
	StrategyWeighted Strategy = "weighted"
)

// Endpoint represents one upstream proxy entry.
type Endpoint struct {
	ID       string
	Address  string // host:port
	Protocol string // vless, vmess, trojan, ss, …
	Priority int
	Weight   int

	mu      sync.RWMutex
	latency time.Duration
	alive   bool
}

// Latency returns the last measured RTT.
func (e *Endpoint) Latency() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.latency
}

// Alive returns whether the endpoint passed its last health check.
func (e *Endpoint) Alive() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.alive
}

// UpdateHealth records the result of a probe.
func (e *Endpoint) UpdateHealth(lat time.Duration, alive bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.latency = lat
	e.alive = alive
}

// Balancer holds the endpoint pool and selects upstreams.
type Balancer struct {
	mu        sync.RWMutex
	endpoints []*Endpoint
	strategy  Strategy
}

// New creates a Balancer with the given strategy.
func New(strategy Strategy) *Balancer {
	return &Balancer{strategy: strategy}
}

// SetEndpoints replaces the entire endpoint pool.
func (b *Balancer) SetEndpoints(eps []*Endpoint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.endpoints = eps
}

// Pick returns the best available endpoint according to the configured strategy.
// Returns nil if no alive endpoints exist.
func (b *Balancer) Pick() *Endpoint {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// TODO: implement latency-sorted, priority, and weighted selection
	for _, ep := range b.endpoints {
		if ep.Alive() {
			return ep
		}
	}
	return nil
}

// DialContext dials the destination through the selected endpoint.
func (b *Balancer) DialContext(network, addr string) (net.Conn, error) {
	// TODO: implement per-protocol dialing (SOCKS5 over endpoint, VLESS, etc.)
	ep := b.Pick()
	if ep == nil {
		return net.Dial(network, addr)
	}
	// Placeholder: direct dial ignoring endpoint for now
	_ = ep
	return net.Dial(network, addr)
}
