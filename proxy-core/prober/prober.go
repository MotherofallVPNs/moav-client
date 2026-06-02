// Package prober measures endpoint reachability and latency.
package prober

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/ibeezhan/moav-client/proxy-core/balancer"
)

const (
	defaultTimeout  = 5 * time.Second
	defaultInterval = 30 * time.Second
	probeTarget     = "8.8.8.8:53" // TCP probe target
)

// Prober periodically checks endpoint health.
type Prober struct {
	balancer *balancer.Balancer
	interval time.Duration
}

// New creates a Prober that feeds results into b.
func New(b *balancer.Balancer) *Prober {
	return &Prober{
		balancer: b,
		interval: defaultInterval,
	}
}

// ProbeAll runs a single probe pass over all endpoints in the balancer.
func (p *Prober) ProbeAll(eps []*balancer.Endpoint) {
	for _, ep := range eps {
		go p.probe(ep)
	}
}

func (p *Prober) probe(ep *balancer.Endpoint) {
	// TODO: dial through the endpoint's protocol rather than directly
	start := time.Now()
	conn, err := net.DialTimeout("tcp", probeTarget, defaultTimeout)
	lat := time.Since(start)

	alive := err == nil
	if conn != nil {
		conn.Close()
	}
	ep.UpdateHealth(lat, alive)
	log.Printf("probe %s (%s): alive=%v lat=%v", ep.ID, ep.Address, alive, lat)
}

// Run starts a background loop that probes every interval until ctx is done.
func (p *Prober) Run(ctx context.Context, eps []*balancer.Endpoint) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.ProbeAll(eps) // initial pass
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.ProbeAll(eps)
		}
	}
}
