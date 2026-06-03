package plugins

import (
	"bufio"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// listCacheEntry is one parsed file's payload.
type listCacheEntry struct {
	domainSuffixes []string         // lowercase, no leading dot
	cidrs          []*net.IPNet
	ips            map[string]bool  // exact IP strings, e.g. "1.2.3.4"
	mtime          time.Time
}

// listCache memoises file → parsed contents. Files are re-read when their
// mtime changes so operators can edit the list at runtime without
// restarting moav-client.
var (
	listCacheMu sync.RWMutex
	listCache   = map[string]*listCacheEntry{}
)

// loadList reads a list file from disk and parses each non-comment line as
// either a CIDR, a bare IP, or a domain suffix. The caller hints whether
// it's looking for domains or IPs; entries of the other kind are kept
// available too so the same file can serve both purposes.
func loadList(path string) *listCacheEntry {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	listCacheMu.RLock()
	c := listCache[path]
	listCacheMu.RUnlock()
	if c != nil && c.mtime.Equal(st.ModTime()) {
		return c
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	out := &listCacheEntry{ips: map[string]bool{}, mtime: st.ModTime()}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// CIDR?
		if strings.Contains(line, "/") {
			if _, n, err := net.ParseCIDR(line); err == nil {
				out.cidrs = append(out.cidrs, n)
				continue
			}
		}
		// Bare IP?
		if ip := net.ParseIP(line); ip != nil {
			out.ips[ip.String()] = true
			continue
		}
		// Domain suffix (strip a leading dot if present).
		out.domainSuffixes = append(out.domainSuffixes, strings.ToLower(strings.TrimPrefix(line, ".")))
	}

	listCacheMu.Lock()
	listCache[path] = out
	listCacheMu.Unlock()
	return out
}

// matchDomainList returns true when host matches (as exact or suffix) any
// domain in the file. Empty file or missing file → no match.
func matchDomainList(host, path string) bool {
	c := loadList(path)
	if c == nil {
		return false
	}
	lower := strings.ToLower(host)
	for _, suf := range c.domainSuffixes {
		if lower == suf || strings.HasSuffix(lower, "."+suf) {
			return true
		}
	}
	return false
}

// matchIPList returns true when host (parsed as an IP) falls within any
// listed CIDR or matches a bare IP entry in the file.
func matchIPList(host, path string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	c := loadList(path)
	if c == nil {
		return false
	}
	if c.ips[ip.String()] {
		return true
	}
	for _, n := range c.cidrs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
