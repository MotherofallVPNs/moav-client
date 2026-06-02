// Package api exposes a REST + WebSocket interface consumed by the web-ui.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/ibeezhan/moav-client/proxy-core/balancer"
	"github.com/ibeezhan/moav-client/proxy-core/prober"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
	"golang.org/x/net/websocket"
)

// Server is the API HTTP server.
type Server struct {
	port     int
	balancer *balancer.Balancer
	prober   *prober.Prober

	// in-memory config store (for POST /api/config).
	cfgMu  sync.RWMutex
	config map[string]interface{}

	// WebSocket hub: broadcast endpoint updates to connected clients.
	hubMu   sync.RWMutex
	clients map[chan []byte]struct{}
}

// New creates an API Server.
func New(port int, b *balancer.Balancer) *Server {
	return &Server{
		port:     port,
		balancer: b,
		prober:   prober.New(),
		config:   map[string]interface{}{},
		clients:  map[chan []byte]struct{}{},
	}
}

// ListenAndServe starts the API server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/probe", s.handleProbe)
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.Handle("/api/ws", websocket.Handler(s.handleWebSocket))

	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	log.Printf("API listening on %s", addr)

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("api listen: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// handleEndpoints returns the current endpoint list as JSON.
func (s *Server) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	eps := s.balancer.Endpoints()
	writeJSON(w, map[string]interface{}{"endpoints": eps})
}

// handleProbe triggers an immediate probe pass and returns updated endpoints.
func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	eps := s.balancer.Endpoints()
	go func() {
		updated := s.prober.ProbeAll(eps)
		s.balancer.SetEndpoints(updated)
		s.broadcast(updated)
	}()

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]interface{}{"status": "probing"})
}

// handleHealth returns a simple health check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{"ok": true})
}

// handleConfig reads or updates the in-memory config.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.cfgMu.RLock()
		cfg := s.config
		s.cfgMu.RUnlock()
		writeJSON(w, cfg)

	case http.MethodPost:
		var incoming map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.cfgMu.Lock()
		for k, v := range incoming {
			s.config[k] = v
		}
		s.cfgMu.Unlock()
		writeJSON(w, map[string]interface{}{"ok": true})

	default:
		http.Error(w, "GET or POST required", http.StatusMethodNotAllowed)
	}
}

// handleWebSocket streams endpoint updates to the connected client.
func (s *Server) handleWebSocket(ws *websocket.Conn) {
	ch := make(chan []byte, 8)
	s.hubMu.Lock()
	s.clients[ch] = struct{}{}
	s.hubMu.Unlock()

	defer func() {
		s.hubMu.Lock()
		delete(s.clients, ch)
		s.hubMu.Unlock()
		ws.Close()
	}()

	// Send current state immediately on connect.
	eps := s.balancer.Endpoints()
	if data, err := json.Marshal(map[string]interface{}{"endpoints": eps}); err == nil {
		websocket.Message.Send(ws, string(data)) //nolint:errcheck
	}

	for msg := range ch {
		if err := websocket.Message.Send(ws, string(msg)); err != nil {
			return
		}
	}
}

// broadcast sends updated endpoints to all WebSocket clients.
func (s *Server) broadcast(eps []subscription.Endpoint) {
	data, err := json.Marshal(map[string]interface{}{"endpoints": eps})
	if err != nil {
		return
	}

	s.hubMu.RLock()
	defer s.hubMu.RUnlock()
	for ch := range s.clients {
		select {
		case ch <- data:
		default: // slow client; skip
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write JSON: %v", err)
	}
}
