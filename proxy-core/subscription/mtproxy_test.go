package subscription

import "testing"

func TestParseMTProxy_Shapes(t *testing.T) {
	cases := []struct {
		uri     string
		address string
		secret  string
	}{
		{"tg://proxy?server=1.2.3.4&port=443&secret=abcdef", "1.2.3.4:443", "abcdef"},
		{"https://t.me/proxy?server=mtp.example.com&port=8443&secret=deadbeef", "mtp.example.com:8443", "deadbeef"},
		{"mtproxy://deadbeef@1.2.3.4:443", "1.2.3.4:443", "deadbeef"},
	}
	for _, c := range cases {
		ep, err := parseMTProxy(c.uri)
		if err != nil {
			t.Errorf("%s: %v", c.uri, err)
			continue
		}
		if ep.Protocol != "mtproxy" {
			t.Errorf("%s: protocol %q", c.uri, ep.Protocol)
		}
		if ep.Address != c.address {
			t.Errorf("%s: address %q want %q", c.uri, ep.Address, c.address)
		}
		if ep.Config["secret"] != c.secret {
			t.Errorf("%s: secret %q want %q", c.uri, ep.Config["secret"], c.secret)
		}
	}
}

func TestParseMTProxy_Rejects(t *testing.T) {
	cases := []string{
		"tg://proxy?server=1.2.3.4",                   // missing port + secret
		"mtproxy://1.2.3.4:443",                       // missing secret
		"mtproxy://secret@host",                       // missing port
	}
	for _, c := range cases {
		if _, err := parseMTProxy(c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}
