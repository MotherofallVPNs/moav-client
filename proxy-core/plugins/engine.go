// Package plugins provides a rule-based decision engine for connection routing.
package plugins

import "sync"

// Decision represents the action to take for a connection request.
type Decision int

const (
	// DecisionProxy routes the connection through a moav proxy endpoint.
	DecisionProxy Decision = iota
	// DecisionDirect bypasses the proxy and connects directly.
	DecisionDirect
	// DecisionBlock drops the connection.
	DecisionBlock
)

// DecisionName converts a Decision to its lowercase API string form.
func DecisionName(d Decision) string {
	switch d {
	case DecisionDirect:
		return "direct"
	case DecisionBlock:
		return "block"
	default:
		return "proxy"
	}
}

// DecisionFromName parses an API string into a Decision; unknown becomes Proxy.
func DecisionFromName(s string) Decision {
	switch s {
	case "direct":
		return DecisionDirect
	case "block":
		return DecisionBlock
	default:
		return DecisionProxy
	}
}

// MatchExpr is a single match criterion.
type MatchExpr struct {
	// Type is one of: "domain", "domain_suffix", "domain_keyword",
	// "ip_cidr", "geoip", "port", "protocol".
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Rule pairs a match expression with a decision. The optional ID is assigned
// by the engine on first insertion so the API can edit/delete by handle.
type Rule struct {
	ID      string    `json:"id,omitempty"`
	Match   MatchExpr `json:"match"`
	Action  Decision  `json:"-"`
	Enabled bool      `json:"enabled"`
	Note    string    `json:"note,omitempty"` // free-form annotation surfaced in the dashboard

	// ActionName is the JSON-facing twin of Action (Decision is an int, ugly to
	// expose). Stays in sync via setActionFromName / engine.SetFromAPI.
	ActionName string `json:"action"`
}

// Engine evaluates an ordered list of rules against connection attributes.
// Evaluation is first-match-wins; if no rule matches, DecisionProxy is returned.
// Engine is safe for concurrent Evaluate() / SetRules() calls — Evaluate runs
// under an RLock so the plugin tab can hot-swap rules without restarting the
// listeners.
type Engine struct {
	mu    sync.RWMutex
	rules []Rule
	// blockDirect, when set, turns every DecisionDirect into DecisionBlock —
	// a kill-switch so nothing ever leaves the host unproxied.
	blockDirect bool
}

// SetBlockDirect toggles the no-direct kill-switch.
func (e *Engine) SetBlockDirect(v bool) {
	e.mu.Lock()
	e.blockDirect = v
	e.mu.Unlock()
}

// BlockDirect reports whether the no-direct kill-switch is on.
func (e *Engine) BlockDirect() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.blockDirect
}

// NewEngine returns an Engine with the given rules. Rules without IDs are
// assigned sequential ones so the API can edit/delete them.
func NewEngine(rules []Rule) *Engine {
	e := &Engine{}
	e.SetRules(rules)
	return e
}

// Rules returns a copy of the current rule list (safe to mutate).
func (e *Engine) Rules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Rule, len(e.rules))
	copy(out, e.rules)
	return out
}

// SetRules atomically replaces the rule list.
func (e *Engine) SetRules(rules []Rule) {
	out := make([]Rule, len(rules))
	for i, r := range rules {
		if r.ID == "" {
			r.ID = genRuleID(i, r)
		}
		if r.ActionName == "" {
			r.ActionName = DecisionName(r.Action)
		} else {
			r.Action = DecisionFromName(r.ActionName)
		}
		out[i] = r
	}
	e.mu.Lock()
	e.rules = out
	e.mu.Unlock()
}

// Evaluate returns the Decision for a connection to host:port with the given
// protocol hint (e.g. "tcp", "udp"). Returns DecisionProxy when no rule matches.
func (e *Engine) Evaluate(host string, port int, protocolHint string) Decision {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, r := range e.rules {
		if !r.Enabled {
			continue
		}
		if matchExpr(r.Match, host, port, protocolHint) {
			if r.Action == DecisionDirect && e.blockDirect {
				return DecisionBlock
			}
			return r.Action
		}
	}
	return DecisionProxy
}

// genRuleID produces a short stable id from the match content + index.
func genRuleID(idx int, r Rule) string {
	return r.Match.Type + ":" + r.Match.Value + ":" + r.ActionName + "#" + itoa(idx)
}

func itoa(i int) string {
	// avoid pulling in strconv just for this
	if i == 0 {
		return "0"
	}
	var buf [16]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
