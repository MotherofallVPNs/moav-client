package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/balancer"
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// newTestMux builds a Server + mux exactly like ListenAndServe but doesn't
// open a TCP listener, so tests run in-process.
func newTestMux(t *testing.T, cfgPath, statePath string, endpoints []subscription.Endpoint) (*Server, http.Handler) {
	t.Helper()
	b := balancer.New(balancer.StrategyLatency)
	b.SetEndpoints(endpoints)
	eng := plugins.NewEngine(nil)

	s := New(0, cfgPath, statePath, b, eng)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/endpoints/", s.handleEndpointPatch)
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/strategy", s.handleStrategy)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/plugins", s.handlePlugins)
	mux.HandleFunc("/api/sources", s.handleSources)
	mux.HandleFunc("/api/sources/", s.handleSourceByName)
	mux.HandleFunc("/api/exposure", s.handleExposure)
	return s, withCORS(withBasicAuth(mux))
}

func TestEndpointPatch_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	statePath := filepath.Join(dir, "state.json")
	os.WriteFile(cfgPath, []byte("proxy: {socks5_port: 1080}\n"), 0o644)

	endpoints := []subscription.Endpoint{
		{ID: "vless:1.2.3.4:443", Protocol: "vless", Address: "1.2.3.4:443", Enabled: true, Priority: 5},
	}
	s, mux := newTestMux(t, cfgPath, statePath, endpoints)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// PATCH: flip enabled to false, priority to 1.
	body := bytes.NewBufferString(`{"enabled": false, "priority": 1}`)
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/endpoints/vless:1.2.3.4:443", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("PATCH status %d: %s", resp.StatusCode, raw)
	}

	// Balancer should reflect the change.
	got := s.balancer.Endpoints()[0]
	if got.Enabled || got.Priority != 1 {
		t.Errorf("balancer not patched: %+v", got)
	}
	// state.json should exist with the new values.
	if _, err := os.Stat(statePath); err != nil {
		t.Errorf("state.json not written after patch: %v", err)
	}
}

func TestStrategySwitch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("proxy: {}\n"), 0o644)
	_, mux := newTestMux(t, filepath.Join(dir, "config.yaml"), filepath.Join(dir, "state.json"), nil)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	for _, s := range []string{"latency", "priority", "weighted"} {
		req, _ := http.NewRequest("POST", ts.URL+"/api/strategy",
			bytes.NewBufferString(`{"strategy":"`+s+`"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != 200 {
			raw, _ := io.ReadAll(resp.Body)
			t.Errorf("strategy %q: %s", s, raw)
		}
	}

	// Bad strategy should 400.
	req, _ := http.NewRequest("POST", ts.URL+"/api/strategy",
		bytes.NewBufferString(`{"strategy":"nope"}`))
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 400 {
		t.Errorf("bad strategy should 400, got %d", resp.StatusCode)
	}
}

func TestExposure_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("proxy: {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte(""), 0o644)

	// The handler reads/writes .env from the current working directory.
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)

	_, mux := newTestMux(t, cfgPath, filepath.Join(dir, "state.json"), nil)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// PUT LAN exposure with auth.
	payload := `{"exposure":"lan","auth":{"username":"u","password":"p"},"dashboard":{"username":"d","password":"dp"}}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/exposure", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT exposure: %s", raw)
	}

	raw, _ := os.ReadFile(".env")
	body := string(raw)
	for _, want := range []string{
		"MOAV_EXPOSURE=lan", "SOCKS5_BIND=0.0.0.0", "SOCKS5_USERNAME=u",
		"SOCKS5_PASSWORD=p", "MOAV_DASHBOARD_USER=d", "MOAV_DASHBOARD_PASS=dp",
	} {
		if !strings.Contains(body, want) {
			t.Errorf(".env missing %q after PUT — body:\n%s", want, body)
		}
	}

	// GET should report back the new exposure mode + masked passwords.
	resp, _ = http.Get(ts.URL + "/api/exposure")
	var got map[string]any
	json.NewDecoder(resp.Body).Decode(&got)
	if got["exposure"] != "lan" {
		t.Errorf("GET exposure: want lan, got %v", got["exposure"])
	}
	if got["auth_password"] == "p" {
		t.Errorf("password not masked: %v", got["auth_password"])
	}
}

func TestBasicAuth_OnlyWhenEnvSet(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("proxy: {}\n"), 0o644)

	// No env → 200.
	_, mux := newTestMux(t, cfgPath, filepath.Join(dir, "state.json"), nil)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	resp, _ := http.Get(ts.URL + "/api/endpoints")
	if resp.StatusCode != 200 {
		t.Errorf("no env set: expected 200, got %d", resp.StatusCode)
	}

	// With env → 401 without auth.
	os.Setenv("MOAV_DASHBOARD_USER", "admin")
	os.Setenv("MOAV_DASHBOARD_PASS", "secret")
	defer os.Unsetenv("MOAV_DASHBOARD_USER")
	defer os.Unsetenv("MOAV_DASHBOARD_PASS")

	_, mux2 := newTestMux(t, cfgPath, filepath.Join(dir, "state.json"), nil)
	ts2 := httptest.NewServer(mux2)
	defer ts2.Close()

	resp, _ = http.Get(ts2.URL + "/api/endpoints")
	if resp.StatusCode != 401 {
		t.Errorf("with env, no auth: expected 401, got %d", resp.StatusCode)
	}

	// /api/healthz should still be 200 even with auth env set.
	resp, _ = http.Get(ts2.URL + "/api/healthz")
	if resp.StatusCode != 200 {
		t.Errorf("/api/healthz with auth env: expected 200, got %d", resp.StatusCode)
	}

	// With correct credentials → 200.
	req, _ := http.NewRequest("GET", ts2.URL+"/api/endpoints", nil)
	req.SetBasicAuth("admin", "secret")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Errorf("correct creds: expected 200, got %d", resp.StatusCode)
	}
}
