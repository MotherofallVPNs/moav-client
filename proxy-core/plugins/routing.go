package plugins

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

// geoipWarned tracks which country codes we've already warned about a missing
// list file for, so the warning fires once per cc instead of per connection.
var geoipWarned sync.Map

// matchExpr evaluates a single MatchExpr against connection attributes.
// It is the shared matching logic used by both Engine and Router.
func matchExpr(m MatchExpr, host string, port int, protocolHint string) bool {
	lower := strings.ToLower(host)
	switch m.Type {
	case "domain":
		return strings.EqualFold(m.Value, host)

	case "domain_suffix":
		suffix := strings.ToLower(m.Value)
		// Match "example.com" against "example.com" or "sub.example.com".
		return lower == suffix || strings.HasSuffix(lower, "."+suffix)

	case "domain_keyword":
		return strings.Contains(lower, strings.ToLower(m.Value))

	case "ip_cidr":
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		_, network, err := net.ParseCIDR(m.Value)
		if err != nil {
			return false
		}
		return network.Contains(ip)

	case "geoip":
		return matchGeoIP(host, strings.ToUpper(m.Value))

	case "domain_list":
		return matchDomainList(host, m.Value)

	case "ip_list":
		return matchIPList(host, m.Value)

	case "port":
		return matchPort(port, m.Value)

	case "protocol":
		return strings.EqualFold(protocolHint, m.Value)
	}
	return false
}

// matchPort checks whether port matches the value, which may be a single
// number ("80") or an inclusive range ("1000-2000").
func matchPort(port int, value string) bool {
	if dash := strings.Index(value, "-"); dash >= 0 {
		lo, err1 := strconv.Atoi(value[:dash])
		hi, err2 := strconv.Atoi(value[dash+1:])
		if err1 != nil || err2 != nil {
			return false
		}
		return port >= lo && port <= hi
	}
	p, err := strconv.Atoi(value)
	if err != nil {
		return false
	}
	return port == p
}

// matchGeoIP checks whether host (as an IP) falls within any CIDR listed in
// geoip/<cc>.txt (one CIDR per line).
//
// NOTE: This is a file-based stub intended for small, manually maintained CIDR
// lists. For production use, replace with a MaxMind mmdb lookup via
// github.com/oschwald/maxminddb-golang.
func matchGeoIP(host, cc string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	path := fmt.Sprintf("geoip/%s.txt", strings.ToLower(cc))
	f, err := os.Open(path)
	if err != nil {
		// File absent → the rule can't match, so it's silently inert. For a
		// BLOCK rule that's a fail-open leak (the operator thinks a region is
		// blocked but traffic flows). We can't safely fail closed for every
		// destination, so warn loudly once per cc to surface the misconfig.
		if _, seen := geoipWarned.LoadOrStore(strings.ToLower(cc), true); !seen {
			log.Printf("plugins: WARN geoip list %q not found — rules matching geoip:%s are INERT (no block/route applied). Populate %s.", path, cc, path)
		}
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, network, err := net.ParseCIDR(line)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// RoutingRule maps one or more MatchExprs (all must match, AND semantics)
// to a Decision.
type RoutingRule struct {
	Match  []MatchExpr
	Action Decision
}

// Router evaluates an ordered list of RoutingRules.
// Each rule requires ALL of its match expressions to be satisfied (AND logic).
// First matching rule wins; when no rule matches, DecisionProxy is returned.
type Router struct {
	Rules []RoutingRule
}

// Evaluate returns the Decision for the connection.
func (r *Router) Evaluate(host string, port int, protocolHint string) Decision {
	for _, rule := range r.Rules {
		if len(rule.Match) == 0 {
			continue
		}
		allMatch := true
		for _, m := range rule.Match {
			if !matchExpr(m, host, port, protocolHint) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return rule.Action
		}
	}
	return DecisionProxy
}
