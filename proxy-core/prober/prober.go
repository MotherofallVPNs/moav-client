// Package prober measures endpoint reachability and latency via TCP connect.
package prober

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
	"golang.org/x/net/proxy"
)

const (
	defaultTimeout  = 10 * time.Second
	defaultInterval = 30 * time.Second
	maxParallel     = 10
)

// Prober measures latency for a slice of Endpoints.
type Prober struct {
	Timeout  time.Duration
	Target   string // host:port tunneled through SOCKS probes; default 1.1.1.1:443
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
// After every pass we emit a single summary log line — INFO when everything
// is healthy, WARN with the failing endpoint names when not — so operators
// can spot trouble in the Debug tab without trawling each per-endpoint line
// (those stay at INFO level because one peer down isn't a system error).
func (p *Prober) ProbeAll(endpoints []subscription.Endpoint) []subscription.Endpoint {
	sem := make(chan struct{}, maxParallel)
	results := make([]subscription.Endpoint, len(endpoints))

	var wg sync.WaitGroup
	var disabledCount int
	for i, ep := range endpoints {
		// Disabled endpoints are passthrough: copy through unchanged so
		// the balancer pool slot stays present but we don't probe them.
		// Otherwise they'd skew the cycle's unhealthy count and waste
		// SOCKS5 dials against ports the user intentionally turned off.
		if !ep.Enabled {
			results[i] = ep
			disabledCount++
			continue
		}
		wg.Add(1)
		go func(idx int, e subscription.Endpoint) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = p.ProbeOne(e)
		}(i, ep)
	}
	wg.Wait()

	// Emit a roll-up so the Debug tab has a single line per cycle showing
	// who's down. We compose the message such that classifyLevel in logbus
	// flags it warn when there are failures. Disabled endpoints are
	// excluded from both numerator and denominator — they're intentionally
	// off, not unhealthy.
	var down []string
	probedCount := 0
	for _, ep := range results {
		if !ep.Enabled {
			continue
		}
		probedCount++
		switch ep.Status {
		case "error", "timeout":
			label := ep.Name
			if label == "" {
				label = ep.ID
			}
			down = append(down, label)
		}
	}
	suffix := ""
	if disabledCount > 0 {
		suffix = fmt.Sprintf(" (%d disabled)", disabledCount)
	}
	if len(down) > 0 {
		log.Printf("probe cycle: WARN %d/%d endpoints unhealthy: %s%s",
			len(down), probedCount, strings.Join(down, ", "), suffix)
	} else if probedCount > 0 {
		log.Printf("probe cycle: all %d endpoints ok%s", probedCount, suffix)
	}

	return results
}

// ProbeOne probes a single endpoint and returns a copy with updated
// LatencyMs and Status fields.
//
// When Config["socks5_addr"] is set (sing-box dialer or sidecar mode), we
// probe that local SOCKS5 port — which validates the whole tunnel reachability
// through sing-box, not just remote TCP connectivity. Otherwise we fall back
// to a raw TCP connect against ep.Address.
func (p *Prober) ProbeOne(ep subscription.Endpoint) subscription.Endpoint {
	updated := ep // copy

	probedAddr := ep.Address
	if socksAddr := ep.Config["socks5_addr"]; socksAddr != "" {
		// SOCKS5 probe: send a CONNECT to a well-known target so the
		// measurement reflects the whole tunnel (sing-box + remote moav
		// server + reachability of the test target), not just the loopback.
		probedAddr = socksAddr
		updated.LatencyMs, updated.Status = socksConnect(socksAddr, p.probeTarget(), p.Timeout)
	} else {
		switch ep.Protocol {
		case "sidecar":
			probedAddr = "127.0.0.1:1080"
			updated.LatencyMs, updated.Status = tcpConnect("127.0.0.1:1080", p.Timeout)
		default:
			updated.LatencyMs, updated.Status = tcpConnect(ep.Address, p.Timeout)
		}
	}

	log.Printf("probe %s via %s: status=%s latency=%dms",
		ep.ID, probedAddr, updated.Status, updated.LatencyMs)
	return updated
}

// probeTarget is the host:port we tunnel to during SOCKS probes. cloudflare-dns
// over 443 is cheap to dial and tolerates random TLS handshakes.
func (p *Prober) probeTarget() string {
	if p.Target != "" {
		return p.Target
	}
	return "1.1.1.1:443"
}

// Run starts a background loop that probes the endpoints returned by fetch
// every interval until ctx is done. fetch is called fresh on each cycle so
// the prober always works against the LIVE balancer pool — that way user
// PATCHes from the dashboard (enable/disable, priority) survive the probe
// loop instead of being clobbered by a stale snapshot.
//
// Updated endpoints are sent on the returned channel after each pass.
func (p *Prober) Run(ctx context.Context, fetch func() []subscription.Endpoint) <-chan []subscription.Endpoint {
	ch := make(chan []subscription.Endpoint, 1)

	go func() {
		defer close(ch)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		send := func() {
			updated := p.ProbeAll(fetch())
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

// socksConnect SOCKS5-dials target through proxyAddr and measures end-to-end
// time. A successful TCP CONNECT through sing-box implies the full chain
// (sing-box outbound -> moav server -> target) is alive.
func socksConnect(proxyAddr, target string, timeout time.Duration) (int64, string) {
	start := time.Now()
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, &net.Dialer{Timeout: timeout})
	if err != nil {
		return time.Since(start).Milliseconds(), "error"
	}
	cdialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		// Fallback to plain Dial.
		conn, derr := dialer.Dial("tcp", target)
		elapsed := time.Since(start).Milliseconds()
		if derr != nil {
			if isTimeout(derr) {
				return elapsed, "timeout"
			}
			return elapsed, "error"
		}
		conn.Close()
		return elapsed, "ok"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, derr := cdialer.DialContext(ctx, "tcp", target)
	elapsed := time.Since(start).Milliseconds()
	if derr != nil {
		if isTimeout(derr) {
			return elapsed, "timeout"
		}
		return elapsed, "error"
	}
	conn.Close()
	return elapsed, "ok"
}
