package plugins

import "strings"

// torrentPorts is the set of well-known BitTorrent ports.
var torrentPorts = map[int]bool{
	6881:  true,
	6882:  true,
	6883:  true,
	6884:  true,
	6885:  true,
	6886:  true,
	6887:  true,
	6888:  true,
	6889:  true,
	51413: true,
}

// trackerDomains is the list of known BitTorrent tracker apex domains.
// Any subdomain of these is also considered a match.
var trackerDomains = []string{
	"tracker.thepiratebay.org",
	"tracker.openbittorrent.com",
	"tracker.opentrackr.org",
	"announce.torrentsmd.com",
	"tracker.leechers-paradise.org",
	"tracker.coppersurfer.tk",
	"exodus.desync.com",
}

// torrentKeywords triggers blocking when found in the hostname combined with
// typical tracker ports (80, 443) or a UDP-like protocol hint.
var torrentKeywords = []string{"torrent", "tracker"}

// TorrentBlocker is a plugin that blocks BitTorrent traffic.
type TorrentBlocker struct {
	Enabled bool
}

// Match returns true when the connection matches BitTorrent heuristics.
func (t *TorrentBlocker) Match(host string, port int, protocolHint string) bool {
	if !t.Enabled {
		return false
	}

	// 1. Well-known BitTorrent ports.
	if torrentPorts[port] {
		return true
	}

	// 2. Known tracker domains (exact or subdomain).
	lower := strings.ToLower(host)
	for _, td := range trackerDomains {
		if lower == td || strings.HasSuffix(lower, "."+td) {
			return true
		}
	}

	// 3. Keyword match combined with tracker-like port/protocol.
	isTrackerPort := port == 80 || port == 443
	isUDP := strings.EqualFold(protocolHint, "udp")
	if isTrackerPort || isUDP {
		for _, kw := range torrentKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}

	return false
}
