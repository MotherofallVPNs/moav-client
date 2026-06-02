// Package api exposes a REST + WebSocket interface consumed by the web-ui.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/ibeezhan/moav-client/proxy-core/balancer"
)

// Server is the API HTTP server.
type Server struct {
	port     int
	balancer *balancer.Balancer
}

// New creates an API Server.
func New(port int, b *balancer.Balancer) *Server {
	return &Server{port: port, balancer: b}
}

// ListenAndServe starts the API server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/probe", s.handleProbe)
	mux.HandleFunc("/healthz", s.handleHealth)

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

// handleEndpoints returns the current endpoint list as JSON.
func (s *Server) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	// TODO: return real endpoint data from the balancer
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"endpoints": []any{},
	})
}

// handleProbe triggers an immediate probe pass.
func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	// TODO: trigger prober
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}
