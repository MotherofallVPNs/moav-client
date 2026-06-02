// Package api exposes a REST + WebSocket interface consumed by the web-ui.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/ibeezhan/moav-client/proxy-core/balancer"
	"github.com/ibeezhan/moav-client/proxy-core/logbus"
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
	"github.com/ibeezhan/moav-client/proxy-core/prober"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
	"golang.org/x/net/websocket"
)

// Server is the API HTTP server.
type Server struct {
	port     int
	balancer *balancer.Balancer
	prober   *prober.Prober
	engine   *plugins.Engine // hot-swappable plugin engine (Plugins tab)

	// in-memory config store (for POST /api/config).
	cfgMu  sync.RWMutex
	config map[string]interface{}

	// WebSocket hub: broadcast endpoint updates to connected clients.
	hubMu   sync.RWMutex
	clients map[chan []byte]struct{}
}

// New creates an API Server.
func New(port int, b *balancer.Balancer, eng *plugins.Engine) *Server {
	return &Server{
		port:     port,
		balancer: b,
		prober:   prober.New(),
		engine:   eng,
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
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/strategy", s.handleStrategy)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/plugins", s.handlePlugins)
	mux.Handle("/api/ws", websocket.Handler(s.handleWebSocket))

	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	log.Printf("API listening on %s", addr)

	srv := &http.Server{Addr: addr, Handler: withCORS(mux)}
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

// handleStats returns per-endpoint counters + active strategy meta. Joins
// the endpoint list with the live balancer stats so the UI can render a
// "Analytics" table without two round-trips.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.balancer.Stats().Snapshot()
	eps := s.balancer.Endpoints()
	byID := make(map[string]int, len(eps))
	for i, ep := range eps {
		byID[ep.ID] = i
	}

	type row struct {
		balancer.EndpointStat
		Name      string `json:"name"`
		Protocol  string `json:"protocol"`
		Address   string `json:"address"`
		Status    string `json:"status"`
		LatencyMs int64  `json:"latency_ms"`
	}
	rows := make([]row, 0, len(stats))
	for _, st := range stats {
		r := row{EndpointStat: st}
		if i, ok := byID[st.ID]; ok {
			r.Name = eps[i].Name
			r.Protocol = eps[i].Protocol
			r.Address = eps[i].Address
			r.Status = eps[i].Status
			r.LatencyMs = eps[i].LatencyMs
		}
		rows = append(rows, r)
	}

	writeJSON(w, map[string]interface{}{
		"strategy": s.balancer.StrategyName(),
		"rows":     rows,
	})
}

// handlePlugins reads or replaces the plugin engine rule list.
// GET  → {rules, templates} for the Plugins tab to render both panes.
// PUT  → replace entire rule list (atomic via Engine.SetRules).
//        Body: {"rules": [{...Rule...}]}
func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]interface{}{
			"rules":     s.engine.Rules(),
			"templates": plugins.Templates,
		})
	case http.MethodPut, http.MethodPost:
		var body struct {
			Rules []plugins.Rule `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.engine.SetRules(body.Rules)
		log.Printf("plugins: replaced rule list (%d rules) via API", len(body.Rules))
		writeJSON(w, map[string]interface{}{"ok": true, "rules": s.engine.Rules()})
	default:
		http.Error(w, "GET or PUT required", http.StatusMethodNotAllowed)
	}
}

// handleLogs returns the in-memory log ring buffer. Optional ?level=
// (comma-separated) filters by levels client-side too, but doing it here
// keeps responses smaller for the initial paint.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	events := logbus.Default.Snapshot()
	if want := r.URL.Query().Get("level"); want != "" {
		allowed := make(map[string]bool, 3)
		for _, l := range strings.Split(want, ",") {
			allowed[strings.TrimSpace(l)] = true
		}
		filtered := events[:0:0]
		for _, ev := range events {
			if allowed[ev.Level] {
				filtered = append(filtered, ev)
			}
		}
		events = filtered
	}
	writeJSON(w, map[string]interface{}{"events": events})
}

// handleStrategy switches the load-balancing strategy at runtime.
// POST {"strategy": "latency" | "priority" | "weighted"}.
func (s *Server) handleStrategy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Strategy string `json:"strategy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	switch body.Strategy {
	case "latency", "priority", "weighted":
		s.balancer.SetStrategy(balancer.Strategy(body.Strategy))
		writeJSON(w, map[string]interface{}{"ok": true, "strategy": body.Strategy})
	default:
		http.Error(w, "strategy must be latency|priority|weighted", http.StatusBadRequest)
	}
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

// handleWebSocket streams endpoint updates AND live log events to the
// connected client. Frames are JSON objects; consumers dispatch on the
// keys present ("endpoints" vs "log").
func (s *Server) handleWebSocket(ws *websocket.Conn) {
	ch := make(chan []byte, 8)
	s.hubMu.Lock()
	s.clients[ch] = struct{}{}
	s.hubMu.Unlock()

	logCh, releaseLog := logbus.Default.Subscribe(64)

	defer func() {
		s.hubMu.Lock()
		delete(s.clients, ch)
		s.hubMu.Unlock()
		releaseLog()
		ws.Close()
	}()

	// Send current state immediately on connect.
	eps := s.balancer.Endpoints()
	if data, err := json.Marshal(map[string]interface{}{"endpoints": eps}); err == nil {
		websocket.Message.Send(ws, string(data)) //nolint:errcheck
	}

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if err := websocket.Message.Send(ws, string(msg)); err != nil {
				return
			}
		case ev := <-logCh:
			frame, _ := json.Marshal(map[string]interface{}{"log": ev})
			if err := websocket.Message.Send(ws, string(frame)); err != nil {
				return
			}
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

// withCORS wraps any handler with permissive CORS. moav-client always runs
// locally and the dashboard is hosted on a different port (3001 in compose,
// 5173 in vite dev), so we accept any origin and let the browser do the rest.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write JSON: %v", err)
	}
}
