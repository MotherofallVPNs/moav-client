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

	// --- "selective app" templates --------------------------------------------
	// These route by DESTINATION, not by originating app — moav-client is a
	// SOCKS5/HTTP proxy and can't see the process (unlike TripMode / WireSock,
	// which run at the OS network layer). They work because these apps talk to
	// distinctive backends; an app riding a shared CDN can't be isolated this
	// way. Domain lists are curated starting points — tune to taste.

	{
		Key:   "block-system-updates",
		Title: "Block OS / system updates",
		Help:  "Drops macOS / iOS (Apple) and Windows software-update domains to save VPN bandwidth. NOTE: while on, the OS can't fetch updates through this proxy at all — flip the rules to `direct` instead if you want updates to work but bypass the VPN.",
		Rules: []Rule{
			// Apple software update (macOS + iOS).
			{Match: MatchExpr{Type: "domain_suffix", Value: "swcdn.apple.com"}, ActionName: "block", Note: "Apple update CDN"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "swscan.apple.com"}, ActionName: "block", Note: "Apple update scan"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "swdist.apple.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "swdownload.apple.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "mesu.apple.com"}, ActionName: "block", Note: "Apple update metadata"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "gdmf.apple.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "updates.cdn-apple.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "updates-http.cdn-apple.com"}, ActionName: "block"},
			// Windows Update.
			{Match: MatchExpr{Type: "domain_suffix", Value: "windowsupdate.com"}, ActionName: "block", Note: "Windows Update"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "update.microsoft.com"}, ActionName: "block"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "delivery.mp.microsoft.com"}, ActionName: "block", Note: "Windows Update delivery"},
		},
	},
	{
		Key:   "direct-zoom",
		Title: "Keep Zoom off the VPN (direct)",
		Help:  "Routes Zoom (meetings + the Zoom updater) directly, bypassing the proxy — better call quality and the exit's location won't affect Zoom. Covers signaling/media over zoom.us; Zoom also uses UDP 3478-3481 / 8801-8810 which a SOCKS5 proxy doesn't carry anyway.",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "zoom.us"}, ActionName: "direct", Note: "Zoom (incl. cdn.zoom.us updater)"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "zoom.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "zoomgov.com"}, ActionName: "direct"},
		},
	},
	{
		Key:   "direct-icloud",
		Title: "Keep iCloud off the VPN (direct)",
		Help:  "Routes iCloud sync / CloudKit / iCloud Drive directly. High-bandwidth, sensitive personal sync with no reason to tunnel it through the exit.",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "icloud.com"}, ActionName: "direct", Note: "iCloud"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "icloud-content.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "apple-cloudkit.com"}, ActionName: "direct", Note: "CloudKit"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "me.com"}, ActionName: "direct"},
		},
	},
	{
		Key:   "direct-cloud-sync",
		Title: "Keep cloud sync off the VPN (direct)",
		Help:  "Routes Dropbox / Google Drive / OneDrive directly — heavy background sync you usually don't want eating VPN bandwidth.",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "dropbox.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "dropboxapi.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "dropboxusercontent.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "drive.google.com"}, ActionName: "direct", Note: "Google Drive (shared googleapis.com not included)"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "onedrive.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "onedrive.live.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "1drv.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "storage.live.com"}, ActionName: "direct"},
		},
	},
	{
		Key:   "direct-streaming",
		Title: "Keep streaming off the VPN (direct)",
		Help:  "Routes Netflix / YouTube / Spotify directly — heavy bandwidth. NOTE: this also forgoes any geo-unblocking for these services (they'll use your real location).",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "netflix.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "nflxvideo.net"}, ActionName: "direct", Note: "Netflix CDN"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "googlevideo.com"}, ActionName: "direct", Note: "YouTube video CDN"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "youtube.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "spotify.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "scdn.co"}, ActionName: "direct", Note: "Spotify CDN"},
		},
	},
	{
		Key:   "direct-game-downloads",
		Title: "Keep game downloads off the VPN (direct)",
		Help:  "Routes Steam / Epic / Blizzard content + downloads directly so multi-GB updates don't run through the exit.",
		Rules: []Rule{
			{Match: MatchExpr{Type: "domain_suffix", Value: "steamcontent.com"}, ActionName: "direct", Note: "Steam content"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "steamstatic.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "steampowered.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "epicgames.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "blizzard.com"}, ActionName: "direct"},
			{Match: MatchExpr{Type: "domain_suffix", Value: "battle.net"}, ActionName: "direct"},
		},
	},
}
