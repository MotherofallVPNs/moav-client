package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/MotherofallVPNs/moav-client/proxy-core/balancer"
	"github.com/MotherofallVPNs/moav-client/proxy-core/plugins"
)

// Resolver looks a hostname up for IP-rule classification. *net.Resolver
// (the host's or container's own DNS) satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// classifyTimeout bounds the local lookup made only to test IP rules.
const classifyTimeout = 5 * time.Second

// router decides a route for a destination and dials it. Both listeners use it,
// so SOCKS5 and HTTP CONNECT apply the same resolution order.
type router struct {
	balancer *balancer.Balancer
	engine   *plugins.Engine
	torrent  *plugins.TorrentBlocker
	resolver Resolver
	// dialDirect opens a direct connection; tests replace it to see the target.
	dialDirect func(ctx context.Context, network, addr string) (net.Conn, error)
}

func newRouter(b *balancer.Balancer, eng *plugins.Engine, tb *plugins.TorrentBlocker) *router {
	var d net.Dialer
	return &router{balancer: b, engine: eng, torrent: tb, resolver: net.DefaultResolver, dialDirect: d.DialContext}
}

// route is a routing decision. addrs is set only when the decision came from
// an IP rule matching a locally resolved hostname: the addresses that rule
// matched, which a direct dial reuses instead of resolving again.
type route struct {
	decision plugins.Decision
	addrs    []netip.Addr
}

// decide routes host:port in this order:
//  1. torrent heuristic and then the rules, on the destination as sent;
//  2. if no rule matched, host is a name and IP rules exist, the name is
//     resolved locally (never through a tunnel) and the rules run again on
//     its addresses, so geoip/ip_cidr can keep an in-country name direct
//     before any endpoint is dialled;
//  3. otherwise the default, proxy.
func (rt *router) decide(ctx context.Context, host string, port int, proto string) route {
	if rt.torrent != nil && rt.torrent.Match(host, port, proto) {
		return route{decision: plugins.DecisionBlock}
	}
	if rt.engine == nil {
		return route{decision: plugins.DecisionProxy}
	}
	if d, _, ok := rt.engine.MatchFirst([]string{host}, port, proto); ok {
		return route{decision: d}
	}
	if _, err := netip.ParseAddr(host); err == nil || rt.resolver == nil || !rt.engine.HasIPRules() {
		return route{decision: plugins.DecisionProxy}
	}

	ctx, cancel := context.WithTimeout(ctx, classifyTimeout)
	defer cancel()
	addrs, err := rt.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		// Nothing to classify, so no IP rule can match: default route. The
		// endpoint resolves the name.
		log.Printf("route: local lookup of %s for IP rules failed (%v); using the default route", host, err)
		return route{decision: plugins.DecisionProxy}
	}
	ips := make([]string, len(addrs))
	for i, a := range addrs {
		ips[i] = a.Unmap().String()
	}
	d, matched, ok := rt.engine.MatchFirst(ips, port, proto)
	if !ok {
		return route{decision: plugins.DecisionProxy}
	}
	r := route{decision: d}
	for _, m := range matched {
		r.addrs = append(r.addrs, netip.MustParseAddr(m))
	}
	return r
}

// dial connects to host:port along r. A proxied hostname is forwarded to the
// endpoint unresolved, even after a local lookup: the local answer was only
// used to classify, and it may be a censor's.
func (rt *router) dial(ctx context.Context, r route, network, host string, port int) (net.Conn, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	switch r.decision {
	case plugins.DecisionBlock:
		return nil, errBlocked
	case plugins.DecisionDirect:
		if len(r.addrs) == 0 {
			return rt.dialDirect(ctx, network, addr)
		}
		// Only the addresses the deciding rule matched, so a direct rule for
		// one range never sends traffic straight to another.
		var errs []error
		for _, a := range r.addrs {
			c, err := rt.dialDirect(ctx, network, netip.AddrPortFrom(a, uint16(port)).String())
			if err == nil {
				return c, nil
			}
			errs = append(errs, err)
		}
		return nil, fmt.Errorf("direct dial %s: %w", addr, errors.Join(errs...))
	default:
		return rt.balancer.DialContext(network, addr)
	}
}

var errBlocked = errors.New("blocked by plugin engine")
