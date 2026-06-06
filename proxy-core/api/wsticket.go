package api

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"time"
)

// WebSocket ticket auth.
//
// Browsers (notably iOS Safari) don't attach cached HTTP basic-auth
// credentials to WebSocket handshake requests, so a basic-auth-protected
// /api/ws gets 401'd and the browser re-prompts. Instead the dashboard fetches
// a short-lived single-use ticket over an authenticated normal request (which
// *does* carry the credentials), then opens the WS with ?ticket=<tok>. /api/ws
// is exempt from basic-auth and validated by the ticket instead — only when a
// dashboard password is configured; otherwise the WS is open like the rest.
type wsTickets struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func newWSTickets() *wsTickets { return &wsTickets{m: map[string]time.Time{}} }

const wsTicketTTL = 30 * time.Second

func (t *wsTickets) issue() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	t.mu.Lock()
	t.m[tok] = time.Now().Add(wsTicketTTL)
	now := time.Now()
	for k, exp := range t.m { // opportunistic GC of expired tickets
		if now.After(exp) {
			delete(t.m, k)
		}
	}
	t.mu.Unlock()
	return tok
}

// consume validates a ticket and removes it (single use).
func (t *wsTickets) consume(tok string) bool {
	if tok == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	exp, ok := t.m[tok]
	if ok {
		delete(t.m, tok)
	}
	return ok && time.Now().Before(exp)
}

// dashboardAuthConfigured reports whether a dashboard/API password is set
// (env or the live .env) — i.e. whether the WS must present a ticket.
func dashboardAuthConfigured() bool {
	pass := os.Getenv("MOAV_DASHBOARD_PASS")
	if v := readEnvKV(".env")["MOAV_DASHBOARD_PASS"]; v != "" {
		pass = v
	}
	return pass != ""
}
