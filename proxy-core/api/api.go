// Package api exposes a REST + WebSocket interface consumed by the web-ui.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/proxy"

	"github.com/ibeezhan/moav-client/proxy-core/backup"
	"github.com/ibeezhan/moav-client/proxy-core/balancer"
	"github.com/ibeezhan/moav-client/proxy-core/bundles"
	"github.com/ibeezhan/moav-client/proxy-core/cmd"
	"github.com/ibeezhan/moav-client/proxy-core/dockerctl"
	"github.com/ibeezhan/moav-client/proxy-core/logbus"
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
	"github.com/ibeezhan/moav-client/proxy-core/prober"
	"github.com/ibeezhan/moav-client/proxy-core/state"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
	"gopkg.in/yaml.v3"
	"golang.org/x/net/websocket"
)

// Server is the API HTTP server.
type Server struct {
	port      int
	cfgPath   string
	statePath string
	balancer  *balancer.Balancer
	prober    *prober.Prober
	engine    *plugins.Engine // hot-swappable plugin engine (Plugins tab)

	// in-memory config store (for POST /api/config).
	cfgMu  sync.RWMutex
	config map[string]interface{}

	// WebSocket hub: broadcast endpoint updates to connected clients.
	hubMu   sync.RWMutex
	clients map[chan []byte]struct{}
}

// New creates an API Server.
func New(port int, cfgPath, statePath string, b *balancer.Balancer, eng *plugins.Engine) *Server {
	return &Server{
		port:      port,
		cfgPath:   cfgPath,
		statePath: statePath,
		balancer:  b,
		prober:    prober.New(),
		engine:    eng,
		config:    map[string]interface{}{},
		clients:   map[chan []byte]struct{}{},
	}
}

// persistEndpointState writes the current balancer pool to state.json so the
// user's Enabled / Priority overrides survive a restart. Called from any
// handler that mutates an endpoint.
func (s *Server) persistEndpointState() {
	if s.statePath == "" {
		return
	}
	st := &state.State{
		LastProbeAt: time.Now(),
		Endpoints:   s.balancer.Endpoints(),
	}
	if err := st.Save(s.statePath); err != nil {
		log.Printf("api: state save: %v", err)
	}
}

// ListenAndServe starts the API server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/endpoints/", s.handleEndpointPatch) // PATCH /api/endpoints/<id>
	mux.HandleFunc("/api/probe", s.handleProbe)
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/strategy", s.handleStrategy)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/plugins", s.handlePlugins)
	mux.HandleFunc("/api/sources", s.handleSources)
	mux.HandleFunc("/api/sources/", s.handleSourceByName) // DELETE /api/sources/<name>
	mux.HandleFunc("/api/sources/reload", s.handleSourcesReload)
	mux.HandleFunc("/api/bundles", s.handleBundleUpload)
	mux.HandleFunc("/api/backup", s.handleBackup)
	mux.HandleFunc("/api/restore", s.handleRestore)
	mux.HandleFunc("/api/flows", s.handleFlows)
	mux.HandleFunc("/api/diag", s.handleDiag)
	mux.HandleFunc("/api/snispoof", s.handleSNISpoof)
	mux.HandleFunc("/api/exposure", s.handleExposure)
	mux.Handle("/api/ws", websocket.Handler(s.handleWebSocket))

	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	log.Printf("API listening on %s", addr)

	srv := &http.Server{Addr: addr, Handler: withCORS(withBasicAuth(mux))}
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

// handleEndpointPatch updates a single endpoint's enabled / priority. For
// endpoints whose protocol is "sidecar", we also try to stop / (re)start the
// matching docker-compose service via the docker socket. The socket mount is
// optional; if it's missing, the dial-pool change still applies and the
// container is left alone.
//
//	PATCH /api/endpoints/<id>
//	body: {"enabled": true|false, "priority": 1..N}
func (s *Server) handleEndpointPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "PATCH required", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/endpoints/")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	var body struct {
		Enabled  *bool `json:"enabled"`
		Priority *int  `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	updated, ok := s.balancer.PatchEndpoint(id, balancer.EndpointPatch{
		Enabled:  body.Enabled,
		Priority: body.Priority,
	})
	if !ok {
		http.Error(w, "no such endpoint", http.StatusNotFound)
		return
	}
	log.Printf("api: patched endpoint %s enabled=%v priority=%d", updated.ID, updated.Enabled, updated.Priority)

	// Side effect: for sidecar endpoints, also stop/start the container.
	if updated.Protocol == "sidecar" && body.Enabled != nil {
		kind := updated.Config["sidecar_kind"]
		go s.controlSidecarContainer(kind, *body.Enabled)
	}

	// Persist so the override survives a restart.
	s.persistEndpointState()
	// Push updated pool to anyone listening.
	s.broadcast(s.balancer.Endpoints())
	writeJSON(w, map[string]interface{}{"ok": true, "endpoint": updated})
}

// controlSidecarContainer is best-effort: failures are logged but don't fail
// the API call (the pool change is what matters for routing correctness).
func (s *Server) controlSidecarContainer(kind string, enable bool) {
	if !dockerctl.Available() {
		log.Printf("dockerctl: socket unavailable; %s container left as-is", kind)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := dockerctl.New()
	service := dockerctl.SidecarDockerService(kind)
	containerID, err := c.FindContainerByService(ctx, service)
	if err != nil {
		log.Printf("dockerctl: find %s: %v", service, err)
		return
	}
	if containerID == "" {
		log.Printf("dockerctl: no container labelled %q (run docker compose --profile %s up to create it)", service, kind)
		return
	}
	if enable {
		if err := c.Start(ctx, containerID); err != nil {
			log.Printf("dockerctl: start %s (%s): %v", service, containerID[:12], err)
			return
		}
		log.Printf("dockerctl: started %s container %s", service, containerID[:12])
	} else {
		if err := c.Stop(ctx, containerID); err != nil {
			log.Printf("dockerctl: stop %s (%s): %v", service, containerID[:12], err)
			return
		}
		log.Printf("dockerctl: stopped %s container %s", service, containerID[:12])
	}
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

// handleVersion is used by the dashboard footer. Returns build version,
// the install host's WAN IP (what someone reaching out to proxy-core's
// own egress sees), the proxy egress IP (what a SOCKS5 client routed
// through localhost:1080 sees), and best-effort country annotations for
// each. Both lookups are cached for ~5 min.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	directIP, directCC := observedDirectIP()
	proxyIP, proxyCC := observedProxyIP(s.balancer)
	writeJSON(w, map[string]interface{}{
		"version":         cmd.Version,
		"commit":          buildCommit,
		"uptime_sec":      int(time.Since(startedAt).Seconds()),
		"public_ip":       directIP, // alias kept for the older footer fetch
		"direct_ip":       directIP,
		"direct_country":  directCC,
		"proxy_ip":        proxyIP,
		"proxy_country":   proxyCC,
	})
}

// buildCommit is wired in at build time via -ldflags "-X .../api.buildCommit=..."
// install.sh can read git rev-parse --short HEAD and pass it through; if it's
// not set the footer just shows "dev".
var buildCommit = "dev"

// startedAt records process start so /api/version can report uptime.
var startedAt = time.Now()

// observedDirectIP returns (ip, country) for the install host's WAN egress —
// i.e. the IP someone reaching out to proxy-core directly would see. Cached
// for ~5 min. Used in the footer as "your install's public IP".
func observedDirectIP() (string, string) {
	return cachedIPLookup(&directIPCache, &directIPMu, func() (string, string) {
		c := &http.Client{Timeout: 4 * time.Second}
		return lookupIPCountry(c, "")
	})
}

// observedProxyIP returns (ip, country) as seen via the balancer — the IP
// world sees when traffic is routed through the SOCKS5 listener. We dial
// the local SOCKS5 on 127.0.0.1:1080 so the lookup goes through whatever
// endpoint the balancer would pick right now.
func observedProxyIP(b *balancer.Balancer) (string, string) {
	return cachedIPLookup(&proxyIPCache, &proxyIPMu, func() (string, string) {
		dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:1080", nil, &net.Dialer{Timeout: 6 * time.Second})
		if err != nil {
			return "", ""
		}
		c := &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				Dial: dialer.Dial,
			},
		}
		_ = b // reserved for future per-endpoint pinning
		return lookupIPCountry(c, "")
	})
}

// lookupIPCountry calls Cloudflare's free trace endpoint
// (https://www.cloudflare.com/cdn-cgi/trace). It's friendly to proxied
// requests, doesn't rate-limit, and returns the country code in the
// `loc=XX` line — exactly what we need for the footer flag emoji.
//
// Response format (text/plain):
//   fl=...
//   ip=1.2.3.4
//   loc=US
//   ...
func lookupIPCountry(c *http.Client, _ string) (string, string) {
	resp, err := c.Get("https://www.cloudflare.com/cdn-cgi/trace")
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", ""
	}
	var ip, cc string
	for _, line := range strings.Split(string(body), "\n") {
		switch {
		case strings.HasPrefix(line, "ip="):
			ip = strings.TrimSpace(strings.TrimPrefix(line, "ip="))
		case strings.HasPrefix(line, "loc="):
			cc = strings.TrimSpace(strings.TrimPrefix(line, "loc="))
		}
	}
	return ip, strings.ToUpper(cc)
}

type ipCacheEntry struct {
	ip      string
	country string
	at      time.Time
}

func cachedIPLookup(cache *ipCacheEntry, mu *sync.Mutex, refresh func() (string, string)) (string, string) {
	mu.Lock()
	defer mu.Unlock()
	if time.Since(cache.at) < 5*time.Minute && cache.ip != "" {
		return cache.ip, cache.country
	}
	ip, cc := refresh()
	if ip != "" {
		cache.ip = ip
		cache.country = cc
		cache.at = time.Now()
	}
	return cache.ip, cache.country
}

var (
	directIPCache ipCacheEntry
	directIPMu    sync.Mutex
	proxyIPCache  ipCacheEntry
	proxyIPMu     sync.Mutex
)

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

// handleSources returns the list of subscription sources currently loaded,
// counting endpoints per source from the live balancer pool. The dashboard's
// Sources tab uses this as a "which moav servers am I connected to" view.
func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	type srcRow struct {
		Name      string   `json:"name"`
		File      string   `json:"file,omitempty"`
		URL       string   `json:"url,omitempty"`
		WGFiles   []string `json:"wireguard_files,omitempty"`
		Endpoints int      `json:"endpoints"`
		Healthy   int      `json:"healthy"`
	}

	rawCfg, err := os.ReadFile(s.configPath())
	if err != nil {
		http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var parsed struct {
		Subscription struct {
			URL            string   `yaml:"url"`
			File           string   `yaml:"file"`
			WireGuardFiles []string `yaml:"wireguard_files"`
			Sources        []struct {
				Name           string   `yaml:"name"`
				URL            string   `yaml:"url"`
				File           string   `yaml:"file"`
				WireGuardFiles []string `yaml:"wireguard_files"`
			} `yaml:"sources"`
		} `yaml:"subscription"`
	}
	if err := yaml.Unmarshal(rawCfg, &parsed); err != nil {
		http.Error(w, "parse config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Tally endpoints by Source.
	endpointsBySource := map[string]int{}
	healthyBySource := map[string]int{}
	for _, ep := range s.balancer.Endpoints() {
		endpointsBySource[ep.Source]++
		if ep.Status == "ok" && ep.Enabled {
			healthyBySource[ep.Source]++
		}
	}

	var rows []srcRow
	if parsed.Subscription.File != "" || parsed.Subscription.URL != "" || len(parsed.Subscription.WireGuardFiles) > 0 {
		rows = append(rows, srcRow{
			Name:      "default",
			File:      parsed.Subscription.File,
			URL:       parsed.Subscription.URL,
			WGFiles:   parsed.Subscription.WireGuardFiles,
			Endpoints: endpointsBySource["default"],
			Healthy:   healthyBySource["default"],
		})
	}
	for _, src := range parsed.Subscription.Sources {
		rows = append(rows, srcRow{
			Name:      src.Name,
			File:      src.File,
			URL:       src.URL,
			WGFiles:   src.WireGuardFiles,
			Endpoints: endpointsBySource[src.Name],
			Healthy:   healthyBySource[src.Name],
		})
	}
	writeJSON(w, map[string]interface{}{
		"sources": rows,
		"note":    "Editing requires moav-client restart to fully reload subscriptions.",
	})
}

// handleSourceByName supports DELETE /api/sources/<name>. Drops the source
// from config.yaml's subscription.sources list (or clears the legacy
// single-source fields if name=="default"). The caller is told to hit
// /api/sources/reload (or restart proxy-core) to actually unload the
// endpoints from the live pool.
func (s *Server) handleSourceByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/sources/")
	if name == "" || name == "reload" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "DELETE required", http.StatusMethodNotAllowed)
		return
	}

	raw, err := os.ReadFile(s.configPath())
	if err != nil {
		http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		http.Error(w, "parse config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sub, _ := root["subscription"].(map[string]any)
	if sub == nil {
		http.Error(w, "no subscription block in config", http.StatusNotFound)
		return
	}

	removed := false
	if name == "default" {
		// Clear the legacy single-source fields.
		if _, ok := sub["file"]; ok {
			sub["file"] = ""
			removed = true
		}
		if _, ok := sub["url"]; ok {
			sub["url"] = ""
			removed = true
		}
		if _, ok := sub["wireguard_files"]; ok {
			sub["wireguard_files"] = []any{}
			removed = true
		}
	} else {
		srcs, _ := sub["sources"].([]any)
		var kept []any
		for _, entry := range srcs {
			m, _ := entry.(map[string]any)
			if m == nil {
				kept = append(kept, entry)
				continue
			}
			if n, _ := m["name"].(string); n == name {
				removed = true
				continue
			}
			kept = append(kept, entry)
		}
		sub["sources"] = kept
	}
	if !removed {
		http.Error(w, "no such source: "+name, http.StatusNotFound)
		return
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		http.Error(w, "marshal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(s.configPath(), out, 0o644); err != nil {
		http.Error(w, "write config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("api: removed source %q from config.yaml — restart to unload its endpoints", name)
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"removed": name,
		"note":    "Source removed from config.yaml. POST /api/sources/reload (or restart proxy-core) to unload its endpoints.",
	})
}

// handleSourcesReload triggers a self-restart of proxy-core via the docker
// socket so the new subscription state takes effect. Falls back to "user
// needs to restart manually" if the socket isn't mounted.
//
// We respond BEFORE issuing the restart so the dashboard gets a clean 200;
// the actual restart happens in a goroutine ~500ms later.
func (s *Server) handleSourcesReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !dockerctl.Available() {
		writeJSON(w, map[string]interface{}{
			"ok":   false,
			"note": "docker socket not mounted; restart proxy-core manually (./moav-client restart) to pick up subscription changes.",
		})
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"note": "Restarting proxy-core to reload subscriptions. The dashboard will reconnect in a few seconds.",
	})
	go func() {
		time.Sleep(500 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		c := dockerctl.New()
		id, err := c.FindContainerByService(ctx, "proxy-core")
		if err != nil || id == "" {
			log.Printf("api: reload: couldn't find proxy-core container: %v", err)
			return
		}
		if err := c.Restart(ctx, id); err != nil {
			log.Printf("api: reload: restart failed: %v", err)
		}
	}()
}

// handleSNISpoof reads / patches the sni_spoof block in config.yaml.
//
//	GET  → {enabled, default_fake_sni, default_utls, ports_used, endpoints[]}
//	PUT  → body any subset of {enabled, default_fake_sni, default_utls}
//	       writes the change to config.yaml; caller needs to /api/sources/reload
//	       (or restart) for it to bind on the wire.
func (s *Server) handleSNISpoof(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, err := os.ReadFile(s.configPath())
		if err != nil {
			http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var root map[string]any
		yaml.Unmarshal(raw, &root)
		section, _ := root["sni_spoof"].(map[string]any)
		if section == nil {
			section = map[string]any{}
		}

		// Tally which live endpoints are currently being routed via the
		// spoofer so the UI shows what's actually active.
		var spoofed []map[string]any
		for _, ep := range s.balancer.Endpoints() {
			if ep.Config["spoof_via"] != "" {
				spoofed = append(spoofed, map[string]any{
					"id":        ep.ID,
					"name":      ep.Name,
					"fake_sni":  ep.Config["fake_sni"],
					"utls":      ep.Config["utls"],
					"spoof_via": ep.Config["spoof_via"],
				})
			}
		}
		writeJSON(w, map[string]any{
			"enabled":          boolOr(section["enabled"], false),
			"default_fake_sni": strOr(section["default_fake_sni"], ""),
			"default_utls":     strOr(section["default_utls"], "chrome"),
			"active_endpoints": spoofed,
		})

	case http.MethodPut, http.MethodPost:
		var body struct {
			Enabled        *bool   `json:"enabled,omitempty"`
			DefaultFakeSNI *string `json:"default_fake_sni,omitempty"`
			DefaultUTLS    *string `json:"default_utls,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		raw, err := os.ReadFile(s.configPath())
		if err != nil {
			http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var root map[string]any
		if err := yaml.Unmarshal(raw, &root); err != nil {
			http.Error(w, "parse config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		section, _ := root["sni_spoof"].(map[string]any)
		if section == nil {
			section = map[string]any{
				"listen_host": "0.0.0.0",
				"dial_host":   "sni-spoof",
				"base_port":   12800,
				"output_path": "data/sni-spoof.json",
			}
			root["sni_spoof"] = section
		}
		if body.Enabled != nil {
			section["enabled"] = *body.Enabled
		}
		if body.DefaultFakeSNI != nil {
			section["default_fake_sni"] = *body.DefaultFakeSNI
		}
		if body.DefaultUTLS != nil {
			section["default_utls"] = *body.DefaultUTLS
		}
		out, _ := yaml.Marshal(root)
		if err := os.WriteFile(s.configPath(), out, 0o644); err != nil {
			http.Error(w, "write: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("api: sni_spoof updated (enabled=%v default_fake_sni=%v)", section["enabled"], section["default_fake_sni"])
		writeJSON(w, map[string]any{
			"ok":   true,
			"note": "config.yaml updated. POST /api/sources/reload (or ./moav-client restart) to apply.",
		})
	default:
		http.Error(w, "GET or PUT required", http.StatusMethodNotAllowed)
	}
}

func boolOr(v any, dflt bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return dflt
}
func strOr(v any, dflt string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return dflt
}

// handleFlows returns the live ring buffer of per-connection records.
func (s *Server) handleFlows(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{"flows": s.balancer.Flows().Snapshot()})
}

// handleDiag runs a connectivity check from proxy-core. Three sub-modes:
//
//	?type=tcp&target=<host:port>             — net.Dial, report success / RTT
//	?type=dns&target=<host>                  — net.LookupHost
//	?type=trace&target=<host:port>           — best-effort traceroute via
//	   shelling out to /usr/sbin/traceroute or /bin/tracepath; falls back
//	   to a series of TCP dials with increasing TTLs if neither is present.
//	?via=<endpoint-id>                       — route the TCP test through
//	   that endpoint's SOCKS5 inbound first (so you can ask "is the moav
//	   server reaching api.example.com?").
func (s *Server) handleDiag(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	if target == "" {
		http.Error(w, "target query required", http.StatusBadRequest)
		return
	}
	kind := r.URL.Query().Get("type")
	via := r.URL.Query().Get("via")

	// For TCP / trace we need host:port. If the user only typed a host or
	// IP, default to :443 (covers 95% of "debug the tunnel" cases) instead
	// of blowing up with net.SplitHostPort "missing port in address".
	if kind == "" || kind == "tcp" || kind == "trace" {
		if _, _, err := net.SplitHostPort(target); err != nil {
			target = target + ":443"
		}
	}

	switch kind {
	case "", "tcp":
		s.diagTCP(w, target, via)
	case "dns":
		// DNS only wants the hostname.
		host := target
		if h, _, err := net.SplitHostPort(target); err == nil {
			host = h
		}
		s.diagDNS(w, host)
	case "trace":
		s.diagTrace(w, target)
	default:
		http.Error(w, "type must be tcp|dns|trace", http.StatusBadRequest)
	}
}

func (s *Server) diagTCP(w http.ResponseWriter, target, via string) {
	start := time.Now()
	var conn net.Conn
	var err error
	if via != "" {
		// Find endpoint, dial through its socks5_addr.
		var pin *subscription.Endpoint
		for _, ep := range s.balancer.Endpoints() {
			if ep.ID == via {
				epCopy := ep
				pin = &epCopy
				break
			}
		}
		if pin == nil {
			http.Error(w, "no such endpoint: "+via, http.StatusNotFound)
			return
		}
		socksAddr := pin.Config["socks5_addr"]
		if socksAddr == "" {
			http.Error(w, "endpoint has no socks5_addr (can't dial through it)", http.StatusBadRequest)
			return
		}
		d, dialErr := proxy.SOCKS5("tcp", socksAddr, nil, &net.Dialer{Timeout: 8 * time.Second})
		if dialErr != nil {
			http.Error(w, "build SOCKS5 dialer: "+dialErr.Error(), http.StatusInternalServerError)
			return
		}
		conn, err = d.Dial("tcp", target)
	} else {
		conn, err = net.DialTimeout("tcp", target, 8*time.Second)
	}
	elapsed := time.Since(start)
	res := map[string]interface{}{
		"type":   "tcp",
		"target": target,
		"via":    via,
		"rtt_ms": elapsed.Milliseconds(),
	}
	if err != nil {
		res["ok"] = false
		res["error"] = err.Error()
	} else {
		conn.Close()
		res["ok"] = true
	}
	writeJSON(w, res)
}

func (s *Server) diagDNS(w http.ResponseWriter, host string) {
	start := time.Now()
	ips, err := net.DefaultResolver.LookupHost(context.Background(), host)
	elapsed := time.Since(start)
	res := map[string]interface{}{
		"type":   "dns",
		"target": host,
		"rtt_ms": elapsed.Milliseconds(),
		"ips":    ips,
	}
	if err != nil {
		res["ok"] = false
		res["error"] = err.Error()
	} else {
		res["ok"] = true
	}
	writeJSON(w, res)
}

func (s *Server) diagTrace(w http.ResponseWriter, target string) {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		host = target
	}
	// Try /usr/bin/traceroute or /bin/tracepath if present in the container.
	for _, bin := range []string{"/usr/bin/traceroute", "/usr/sbin/traceroute", "/bin/tracepath", "/usr/bin/tracepath"} {
		if _, statErr := os.Stat(bin); statErr == nil {
			cmd := exec.CommandContext(context.Background(), bin, "-m", "12", host)
			out, runErr := cmd.CombinedOutput()
			writeJSON(w, map[string]interface{}{
				"type":   "trace",
				"target": target,
				"binary": bin,
				"output": string(out),
				"ok":     runErr == nil,
				"error": func() string {
					if runErr != nil {
						return runErr.Error()
					}
					return ""
				}(),
			})
			return
		}
	}
	// Fallback: TCP-based "trace" via increasing TTLs. We just do TCP probes
	// at TTL=1..8 to surface which hops are reachable from us.
	results := []map[string]interface{}{}
	for ttl := 1; ttl <= 8; ttl++ {
		start := time.Now()
		d := net.Dialer{Timeout: 1500 * time.Millisecond, Control: makeTTLControl(ttl)}
		c, err := d.Dial("tcp", target)
		row := map[string]interface{}{
			"ttl":    ttl,
			"rtt_ms": time.Since(start).Milliseconds(),
		}
		if err != nil {
			row["error"] = err.Error()
		} else {
			row["ok"] = true
			row["peer"] = c.RemoteAddr().String()
			c.Close()
		}
		results = append(results, row)
	}
	writeJSON(w, map[string]interface{}{
		"type":     "trace",
		"target":   target,
		"binary":   "(none — using TCP-TTL fallback)",
		"hops":     results,
	})
}

// makeTTLControl sets the IP_TTL socket option before connect so we can do a
// crude traceroute by hand when no traceroute binary is on PATH.
func makeTTLControl(ttl int) func(string, string, syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		var sockerr error
		err := c.Control(func(fd uintptr) {
			sockerr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TTL, ttl)
		})
		if err != nil {
			return err
		}
		return sockerr
	}
}

// handleBackup writes a zip of config.yaml + data/ + .env to the response.
// Runtime artifacts (state.json, generated sing-box/xray configs, sidecar
// configs) are excluded — they regenerate on startup. The result is suitable
// for moving an install to another box: drop it into the target and start.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	cwd, _ := os.Getwd()
	zipBytes, err := backup.Create(cwd)
	if err != nil {
		http.Error(w, "backup: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=moav-client-backup-%s.zip", time.Now().UTC().Format("20060102-150405")))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipBytes)))
	w.Write(zipBytes) //nolint:errcheck
}

// handleRestore accepts a multipart upload of a backup zip and extracts it
// over the current install. Caller is told to restart for the new
// config.yaml to take effect.
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("backup")
	if err != nil {
		http.Error(w, "missing 'backup' file field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	zipBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read upload: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cwd, _ := os.Getwd()
	n, err := backup.Restore(cwd, zipBytes)
	if err != nil {
		http.Error(w, "restore: "+err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("api: restored %d files from backup zip — restart to apply", n)
	writeJSON(w, map[string]interface{}{
		"ok":            true,
		"files_restored": n,
		"note":           "Run /api/sources/reload (or ./moav-client restart) to load the restored config.",
	})
}

// handleBundleUpload accepts a multipart .zip upload, extracts it under
// data/<name>/ via the bundles package, and patches config.yaml to add a
// new subscription source. Caller's responsibility to restart moav-client
// to actually load the new endpoints (we surface that in the response).
//
//	POST /api/bundles
//	form-data: bundle=<zip>  (and optional: name=<friendly-name>)
func (s *Server) handleBundleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil { // 64 MB cap
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("bundle")
	if err != nil {
		http.Error(w, "missing 'bundle' file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	zipBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read upload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	requestedName := strings.TrimSpace(r.FormValue("name"))
	if requestedName == "" {
		// Fall back to the uploaded filename minus extension.
		base := filepath.Base(hdr.Filename)
		requestedName = strings.TrimSuffix(base, filepath.Ext(base))
	}

	res, err := bundles.Extract(zipBytes, "data", requestedName)
	if err != nil {
		http.Error(w, "extract: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Patch config.yaml to add the new source. Best-effort; we use
	// gopkg.in/yaml.v3 to preserve as much structure as we can but the
	// resulting file may have re-ordered keys.
	if err := s.addSourceToConfig(res); err != nil {
		log.Printf("api: bundle extracted but config.yaml patch failed: %v", err)
		writeJSON(w, map[string]interface{}{
			"ok":      true,
			"result":  res,
			"warning": "extracted but config.yaml not updated: " + err.Error(),
			"note":    "Add the source manually under subscription.sources, then restart moav-client.",
		})
		return
	}

	log.Printf("api: imported bundle %q (%d files) — restart proxy-core to load",
		res.Name, len(res.Files))
	writeJSON(w, map[string]interface{}{
		"ok":     true,
		"result": res,
		"note":   "Bundle imported and registered in config.yaml. Restart moav-client (or docker compose restart proxy-core) to load the new endpoints.",
	})
}

// addSourceToConfig appends a new subscription.sources entry corresponding
// to the just-extracted bundle. If masterdns parameters were detected, also
// updates sidecars.masterdns.config (without enabling — user decides).
func (s *Server) addSourceToConfig(res *bundles.Result) error {
	raw, err := os.ReadFile(s.configPath())
	if err != nil {
		return err
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return err
	}

	sub, _ := root["subscription"].(map[string]any)
	if sub == nil {
		sub = map[string]any{}
		root["subscription"] = sub
	}
	srcs, _ := sub["sources"].([]any)

	entry := map[string]any{"name": res.Name}
	if res.SubscriptionPath != "" {
		entry["file"] = relativeIfPossible(res.SubscriptionPath)
	}
	if res.WireGuardConfPath != "" {
		entry["wireguard_files"] = []any{relativeIfPossible(res.WireGuardConfPath)}
	}
	srcs = append(srcs, entry)
	sub["sources"] = srcs

	if res.MasterDNSDomain != "" && res.MasterDNSKey != "" {
		sidecars, _ := root["sidecars"].(map[string]any)
		if sidecars == nil {
			sidecars = map[string]any{}
			root["sidecars"] = sidecars
		}
		md, _ := sidecars["masterdns"].(map[string]any)
		if md == nil {
			md = map[string]any{}
			sidecars["masterdns"] = md
		}
		mdCfg, _ := md["config"].(map[string]any)
		if mdCfg == nil {
			mdCfg = map[string]any{}
			md["config"] = mdCfg
		}
		mdCfg["domain"] = res.MasterDNSDomain
		mdCfg["key"] = res.MasterDNSKey
		if res.MasterDNSMethod != "" {
			mdCfg["method"] = res.MasterDNSMethod
		}
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	// In-place write (bind-mount can't atomic-rename).
	return os.WriteFile(s.configPath(), out, 0o644)
}

// handleExposure reads/writes the .env file that docker-compose uses to
// decide which interface to bind proxy-core's host ports to. Values:
//   - "loopback" → 127.0.0.1:1080 / 127.0.0.1:8081 (default)
//   - "lan"      → 0.0.0.0:1080  / 0.0.0.0:8081   (visible to anything on the LAN)
//   - "public"   → same as lan; the user's firewall is what makes it public
//
// PUT body: {"exposure": "loopback"|"lan"|"public",
//             "auth": {"username": "...", "password": "..."}}
// Auth is optional and only meaningfully strict for lan/public.
func (s *Server) handleExposure(w http.ResponseWriter, r *http.Request) {
	envPath := ".env"

	switch r.Method {
	case http.MethodGet:
		cur := readEnvKV(envPath)
		writeJSON(w, map[string]interface{}{
			"exposure":          defaultStr(cur["MOAV_EXPOSURE"], "loopback"),
			"socks5_bind":       defaultStr(cur["SOCKS5_BIND"], "127.0.0.1"),
			"http_bind":         defaultStr(cur["HTTP_BIND"], "127.0.0.1"),
			"auth_username":     cur["SOCKS5_USERNAME"],
			"auth_password":     maskSecret(cur["SOCKS5_PASSWORD"]),
			"dashboard_user":    cur["MOAV_DASHBOARD_USER"],
			"dashboard_pass":    maskSecret(cur["MOAV_DASHBOARD_PASS"]),
			"note":              "After changing exposure, run: docker compose up -d --force-recreate proxy-core",
		})

	case http.MethodPost, http.MethodPut:
		var body struct {
			Exposure string `json:"exposure"`
			Auth     struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"auth"`
			Dashboard struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"dashboard"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		bindAddr := "127.0.0.1"
		switch body.Exposure {
		case "", "loopback":
			body.Exposure = "loopback"
			bindAddr = "127.0.0.1"
		case "lan", "public":
			bindAddr = "0.0.0.0"
		default:
			http.Error(w, "exposure must be loopback|lan|public", http.StatusBadRequest)
			return
		}
		kv := readEnvKV(envPath)
		kv["MOAV_EXPOSURE"] = body.Exposure
		kv["SOCKS5_BIND"] = bindAddr
		kv["HTTP_BIND"] = bindAddr
		kv["UI_BIND"] = bindAddr
		if body.Auth.Username != "" {
			kv["SOCKS5_USERNAME"] = body.Auth.Username
		}
		if body.Auth.Password != "" {
			kv["SOCKS5_PASSWORD"] = body.Auth.Password
		}
		if body.Dashboard.Username != "" {
			kv["MOAV_DASHBOARD_USER"] = body.Dashboard.Username
		}
		if body.Dashboard.Password != "" {
			kv["MOAV_DASHBOARD_PASS"] = body.Dashboard.Password
		}
		if err := writeEnvKV(envPath, kv); err != nil {
			http.Error(w, "write .env: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("api: set proxy exposure to %s (bind %s) in %s", body.Exposure, bindAddr, envPath)
		writeJSON(w, map[string]interface{}{
			"ok":       true,
			"exposure": body.Exposure,
			"bind":     bindAddr,
			"note":     "Run: docker compose up -d --force-recreate proxy-core web-ui to apply.",
		})
	default:
		http.Error(w, "GET or PUT required", http.StatusMethodNotAllowed)
	}
}

func readEnvKV(path string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		out[strings.TrimSpace(line[:eq])] = strings.TrimSpace(line[eq+1:])
	}
	return out
}

func writeEnvKV(path string, kv map[string]string) error {
	var sb strings.Builder
	sb.WriteString("# moav-client environment — managed by /api/exposure. Edit by hand if you prefer.\n")
	for _, k := range []string{"MOAV_EXPOSURE", "SOCKS5_BIND", "HTTP_BIND", "API_BIND", "UI_BIND", "SOCKS5_USERNAME", "SOCKS5_PASSWORD", "MOAV_DASHBOARD_USER", "MOAV_DASHBOARD_PASS"} {
		if v, ok := kv[k]; ok && v != "" {
			sb.WriteString(k)
			sb.WriteByte('=')
			sb.WriteString(v)
			sb.WriteByte('\n')
		}
	}
	// .env is bind-mounted into the container in compose, so an atomic
	// tmp+rename fails with "device or resource busy" — docker holds the
	// inode for the mount. Open with O_TRUNC and overwrite in place instead.
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return strings.Repeat("•", len(s))
	}
	return s[:2] + strings.Repeat("•", len(s)-4) + s[len(s)-2:]
}

func relativeIfPossible(abs string) string {
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return "./" + rel
		}
	}
	return abs
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

// handleConfig serves the on-disk YAML config so the dashboard's Config tab
// can show the user what's actually loaded. POST writes the bytes back
// (atomically). The in-memory map remains as a legacy free-form store for
// callers that want it under a "_map" key.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, err := os.ReadFile(s.configPath())
		if err != nil {
			http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"path": s.configPath(),
			"yaml": string(raw),
		})

	case http.MethodPost, http.MethodPut:
		var body struct {
			YAML string `json:"yaml"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.YAML == "" {
			http.Error(w, "yaml field required", http.StatusBadRequest)
			return
		}
		path := s.configPath()
		// Bind-mounted files can't be atomically renamed in Docker (device or
		// resource busy on the rename). Overwrite in place; this is fine
		// because the file is only ever written from one caller.
		if err := os.WriteFile(path, []byte(body.YAML), 0o644); err != nil {
			http.Error(w, "write config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("api: wrote %d bytes to %s (restart required to apply)", len(body.YAML), path)
		writeJSON(w, map[string]interface{}{
			"ok":   true,
			"note": "saved to disk — restart moav-client (or docker compose restart proxy-core) to apply",
		})

	default:
		http.Error(w, "GET or POST required", http.StatusMethodNotAllowed)
	}
}

// configPath returns the on-disk YAML config path. Default matches main.go.
func (s *Server) configPath() string {
	if s.cfgPath != "" {
		return s.cfgPath
	}
	return "config.yaml"
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

// withBasicAuth gates the API behind HTTP basic auth when MOAV_DASHBOARD_USER
// + MOAV_DASHBOARD_PASS are set. When unset (the default for loopback
// installs) every request passes through. /api/healthz is always reachable
// so liveness probes from outside don't break.
func withBasicAuth(h http.Handler) http.Handler {
	user := os.Getenv("MOAV_DASHBOARD_USER")
	pass := os.Getenv("MOAV_DASHBOARD_PASS")
	if user == "" || pass == "" {
		return h
	}
	expected := user + ":" + pass
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/healthz" || r.Method == http.MethodOptions {
			h.ServeHTTP(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || (u+":"+p) != expected {
			w.Header().Set("WWW-Authenticate", `Basic realm="moav-client"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// withCORS wraps any handler with permissive CORS. moav-client always runs
// locally and the dashboard is hosted on a different port (3001 in compose,
// 5173 in vite dev), so we accept any origin and let the browser do the rest.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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
