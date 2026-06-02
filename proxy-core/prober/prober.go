// Package prober measures endpoint reachability and latency via TCP connect.
package prober

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

const (
	defaultTimeout  = 10 * time.Second
	defaultInterval = 30 * time.Second
	maxParallel     = 10
)

// Prober measures latency for a slice of Endpoints.
type Prober struct {
	Timeout  time.Duration
	interval time.Duration
}

// New returns a Prober with default settings.
func New() *Prober {
	return &Prober{
		Timeout:  defaultTimeout,
		interval: defaultInterval,
	}
}

// ProbeAll probes all endpoints concurrently (max 10 parallel) and returns
// the updated slice. The input slice is not modified; a new slice is returned.
func (p *Prober) ProbeAll(endpoints []subscription.Endpoint) []subscription.Endpoint {
	sem := make(chan struct{}, maxParallel)
	results := make([]subscription.Endpoint, len(endpoints))

	var wg sync.WaitGroup
	for i, ep := range endpoints {
		wg.Add(1)
		go func(idx int, e subscription.Endpoint) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = p.ProbeOne(e)
		}(i, ep)
	}
	wg.Wait()
	return results
}

// ProbeOne probes a single endpoint and returns a copy with updated
// LatencyMs and Status fields.
func (p *Prober) ProbeOne(ep subscription.Endpoint) subscription.Endpoint {
	updated := ep // copy

	switch ep.Protocol {
	case "wireguard":
		// WireGuard: TCP reachability only (no QUIC/UDP support here).
		updated.LatencyMs, updated.Status = tcpConnect(ep.Address, p.Timeout)
	case "hysteria2":
		// Hysteria2 uses QUIC; fall back to TCP connect for reachability.
		updated.LatencyMs, updated.Status = tcpConnect(ep.Address, p.Timeout)
	case "sidecar":
		// Connect to the local SOCKS5 port exposed by the sidecar.
		socksAddr := ep.Config["socks5_addr"]
		if socksAddr == "" {
			socksAddr = "127.0.0.1:1080"
		}
		updated.LatencyMs, updated.Status = tcpConnect(socksAddr, p.Timeout)
	default:
		// vless, vmess, trojan, ss, tuic: TCP connect to host:port.
		updated.LatencyMs, updated.Status = tcpConnect(ep.Address, p.Timeout)
	}

	log.Printf("probe %s (%s): status=%s latency=%dms",
		ep.ID, ep.Address, updated.Status, updated.LatencyMs)
	return updated
}

// Run starts a background loop that probes eps every interval until ctx is done.
// Updated endpoints are sent on the returned channel after each pass.
func (p *Prober) Run(ctx context.Context, eps []subscription.Endpoint) <-chan []subscription.Endpoint {
	ch := make(chan []subscription.Endpoint, 1)

	go func() {
		defer close(ch)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		send := func() {
			updated := p.ProbeAll(eps)
			select {
			case ch <- updated:
			default:
			}
		}

		send() // initial pass
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				send()
			}
		}
	}()

	return ch
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// tcpConnect dials addr via TCP and returns (latencyMs, status).
// status is one of "ok", "timeout", "error".
func tcpConnect(addr string, timeout time.Duration) (int64, string) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		if isTimeout(err) {
			return elapsed, "timeout"
		}
		return elapsed, "error"
	}
	conn.Close()
	return elapsed, "ok"
}

// isTimeout checks whether an error is a network timeout.
func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}
