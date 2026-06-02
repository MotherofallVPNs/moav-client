// Package plugins provides a rule-based decision engine for connection routing.
package plugins

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

// MatchExpr is a single match criterion.
type MatchExpr struct {
	// Type is one of: "domain", "domain_suffix", "domain_keyword",
	// "ip_cidr", "geoip", "port", "protocol".
	Type  string
	Value string
}

// Rule pairs a match expression with a decision.
type Rule struct {
	Match  MatchExpr
	Action Decision
}

// Engine evaluates an ordered list of rules against connection attributes.
// Evaluation is first-match-wins; if no rule matches, DecisionProxy is returned.
type Engine struct {
	Rules []Rule
}

// NewEngine returns an Engine with the given rules.
func NewEngine(rules []Rule) *Engine {
	return &Engine{Rules: rules}
}

// Evaluate returns the Decision for a connection to host:port with the given
// protocol hint (e.g. "tcp", "udp"). Returns DecisionProxy when no rule matches.
func (e *Engine) Evaluate(host string, port int, protocolHint string) Decision {
	for _, r := range e.Rules {
		if matchExpr(r.Match, host, port, protocolHint) {
			return r.Action
		}
	}
	return DecisionProxy
}
