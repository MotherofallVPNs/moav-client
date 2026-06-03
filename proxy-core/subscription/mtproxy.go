package subscription

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// parseMTProxy accepts three shapes:
//
//	tg://proxy?server=...&port=...&secret=...
//	mtproxy://secret@server:port
//	https://t.me/proxy?server=...&port=...&secret=...
//
// All carry the same three knobs (server, port, secret); MTProxy doesn't
// have other transport options.
func parseMTProxy(uri string) (Endpoint, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Endpoint{}, fmt.Errorf("mtproxy: parse: %w", err)
	}
	q := u.Query()
	var server, port, secret string
	switch {
	case strings.HasPrefix(uri, "mtproxy://"):
		// mtproxy://secret@server:port
		secret = u.User.Username()
		server = u.Hostname()
		port = u.Port()
	default:
		server = q.Get("server")
		port = q.Get("port")
		secret = q.Get("secret")
	}
	if server == "" || port == "" || secret == "" {
		return Endpoint{}, fmt.Errorf("mtproxy: need server, port, secret")
	}
	addr := net.JoinHostPort(server, port)
	return Endpoint{
		ID:       genID("mtproxy", addr),
		Protocol: "mtproxy",
		Name:     u.Fragment,
		Address:  addr,
		RawURI:   uri,
		Config:   map[string]string{"secret": secret},
		Enabled:  true,
		LatencyMs: -1,
		Status:   "unknown",
	}, nil
}
