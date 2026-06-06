package plugins

// Template is one curated rule (or rule group) the dashboard can offer
// users as a one-click "add this". They're disabled-by-default and carry a
// human-readable Note so the Plugins tab can display rationale alongside
// the technical match.
type Template struct {
	Key   string `json:"key"`         // stable identifier ("block-tracker-domains")
	Title string `json:"title"`       // short label for UI
	Help  string `json:"help"`        // longer explanation / source citation
	Rules []Rule `json:"rules"`       // one or more rules; each disabled by default
}

// Templates is the curated catalog returned by GET /api/plugins.
// New entries here automatically show up in the dashboard's "Add from
// template" picker.
var Templates = []Template{
	{
		Key:   "lan-direct",
		Title: "Direct dial for LAN ranges",
		Help:  "Skips the proxy for RFC1918 / link-local destinations (matched as IP literals — the normal way LAN devices are addressed). Practically required so internal services (gateway UIs, NAS, printers) stay reachable when SOCKS5 is set system-wide.",
		Rules: []Rule{
			{Match: MatchExpr{Type: "ip_cidr", Value: "10.0.0.0/8"}, ActionName: "direct", Note: "RFC1918 (private)"},
			{Match: MatchExpr{Type: "ip_cidr", Value: "172.16.0.0/12"}, ActionName: "direct", Note: "RFC1918 (private)"},
			{Match: MatchExpr{Type: "ip_cidr", Value: "192.168.0.0/16"}, ActionName: "direct", Note: "RFC1918 (private)"},
			{Match: MatchExpr{Type: "ip_cidr", Value: "127.0.0.0/8"}, ActionName: "direct", Note: "loopback"},
			{Match: MatchExpr{Type: "ip_cidr", Value: "169.254.0.0/16"}, ActionName: "direct", Note: "link-local"},
		},
	},
	{
		Key:   "block-known-trackers",
		Title: "Block well-known BitTorrent trackers",
		Help:  "Hard-blocks a curated list of trackers. Complements the heuristic TorrentBlocker (which catches port + keyword patterns).",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "tracker.thepiratebay.org"}, ActionName: "block", Note: "TPB tracker"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "tracker.openbittorrent.com"}, ActionName: "block", Note: "OpenBitTorrent"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "tracker.opentrackr.org"}, ActionName: "block", Note: "OpenTrackr"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "exodus.desync.com"}, ActionName: "block", Note: "Exodus tracker"},
		},
	},
	{
		Key:   "block-ad-networks",
		Title: "Block common ad / tracking domains",
		Help:  "A short, conservative starter list (Google Ads + DoubleClick + adservice). For exhaustive coverage point a real DNS sinkhole (Pi-hole, AdGuard) at moav-client's DNS plugin.",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "doubleclick.net"}, ActionName: "block", Note: "Google DoubleClick"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "googleadservices.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "googlesyndication.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "adservice.google.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "scorecardresearch.com"}, ActionName: "block"},
		},
	},
	{
		Key:   "block-telemetry",
		Title: "Block opt-out telemetry endpoints",
		Help:  "Targets pings most OSes / IDEs make even after settings are toggled off. Conservative — only widely-recognised endpoints.",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "telemetry.microsoft.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "vortex.data.microsoft.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "watson.telemetry.microsoft.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "incoming.telemetry.mozilla.org"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "telemetry.jetbrains.com"}, ActionName: "block"},
		},
	},
	{
		Key:   "force-tls-only",
		Title: "Block plain HTTP (port 80)",
		Help:  "Drops any DecisionProxy attempt to TCP port 80. Encourages HTTPS for everything else (note: this also blocks plain HTTP healthchecks).",
		Rules: []Rule{
			{Match: MatchExpr{Type: "port", Value: "80"}, ActionName: "block", Note: "no plaintext HTTP"},
		},
	},
	{
		Key:   "direct-anthropic",
		Title: "Direct dial Anthropic / Claude domains",
		Help:  "Sample: prefer direct path for Anthropic APIs. Useful when you want LLM calls outside the proxy budget.",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "anthropic.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "claude.ai"}, ActionName: "direct"},
		},
	},
}
