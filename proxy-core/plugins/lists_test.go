package plugins

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatchDomainList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.txt")
	os.WriteFile(path, []byte(`# a comment
example.com
.tracker.example.org
gov.uk
`), 0o644)

	cases := []struct {
		host string
		want bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"notexample.com", false},
		{"tracker.example.org", true},
		{"www.tracker.example.org", true},
		{"gov.uk", true},
		{"hmrc.gov.uk", true},
		{"random.org", false},
	}
	for _, c := range cases {
		if got := matchDomainList(c.host, path); got != c.want {
			t.Errorf("matchDomainList(%q): want %v got %v", c.host, c.want, got)
		}
	}
}

func TestMatchIPList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ips.txt")
	os.WriteFile(path, []byte(`# CIDR + bare IPs
192.168.0.0/16
8.8.8.8
2606:4700::/32
`), 0o644)

	cases := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.100", true},
		{"192.168.0.0", true},
		{"10.0.0.1", false},
		{"8.8.8.8", true},
		{"8.8.4.4", false},
		{"2606:4700:1:2::abcd", true},
		{"2001:db8::1", false},
		{"not-an-ip", false},
	}
	for _, c := range cases {
		if got := matchIPList(c.ip, path); got != c.want {
			t.Errorf("matchIPList(%q): want %v got %v", c.ip, c.want, got)
		}
	}
}

func TestListCache_RefreshesOnMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.txt")
	os.WriteFile(path, []byte("first.com\n"), 0o644)
	if !matchDomainList("first.com", path) {
		t.Fatal("first.com should match on initial load")
	}
	// Force a different mtime by sleeping briefly and rewriting.
	os.Chtimes(path, time.Time{}, time.Time{}) // touch
	os.WriteFile(path, []byte("second.com\n"), 0o644)
	if !matchDomainList("second.com", path) {
		t.Error("cache should have refreshed after file change")
	}
}
