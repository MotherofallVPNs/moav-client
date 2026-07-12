package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

func TestLoad_MissingFile_ReturnsEmptyState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load of missing file should not error, got: %v", err)
	}
	if s == nil {
		t.Fatal("Load returned nil state")
	}
	if len(s.Endpoints) != 0 {
		t.Errorf("expected zero endpoints, got %d", len(s.Endpoints))
	}
	if !s.LastProbeAt.IsZero() {
		t.Errorf("expected zero LastProbeAt, got %v", s.LastProbeAt)
	}
}

func TestLoad_CorruptJSON_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error for corrupt JSON, got nil")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	// Truncate to seconds — JSON time round-trips at RFC3339 nanosecond
	// precision, but we compare with Equal so a whole time is safest.
	now := time.Now().UTC().Truncate(time.Second)
	orig := &State{
		LastProbeAt: now,
		Endpoints: []subscription.Endpoint{
			{
				ID:        "sidecar:tor",
				Protocol:  "sidecar",
				Name:      "sidecar-tor",
				Address:   "tor:9150",
				RawURI:    "sidecar://tor:9150",
				Priority:  5,
				Enabled:   true,
				LatencyMs: 42,
				Status:    "ok",
				Config:    map[string]string{"socks5_addr": "tor:9150"},
			},
		},
	}
	if err := orig.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.LastProbeAt.Equal(orig.LastProbeAt) {
		t.Errorf("LastProbeAt mismatch: want %v, got %v", orig.LastProbeAt, got.LastProbeAt)
	}
	if len(got.Endpoints) != 1 {
		t.Fatalf("want 1 endpoint, got %d", len(got.Endpoints))
	}
	ep := got.Endpoints[0]
	if ep.ID != "sidecar:tor" || ep.Address != "tor:9150" || ep.LatencyMs != 42 || ep.Status != "ok" {
		t.Errorf("endpoint round-trip mismatch: %+v", ep)
	}
	if ep.Config["socks5_addr"] != "tor:9150" {
		t.Errorf("config map not preserved: %+v", ep.Config)
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	// Save should create any missing parent directories.
	path := filepath.Join(t.TempDir(), "nested", "deeper", "state.json")
	s := &State{LastProbeAt: time.Now()}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file created at %s: %v", path, err)
	}
}

func TestSave_NoLingeringTmpFile(t *testing.T) {
	// Atomic write goes through a .tmp file that must be renamed away.
	path := filepath.Join(t.TempDir(), "state.json")
	s := &State{}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected no lingering .tmp file, stat err=%v", err)
	}
}

func TestSaveLoad_EmptyState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	orig := &State{}
	if err := orig.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Endpoints) != 0 {
		t.Errorf("expected zero endpoints, got %d", len(got.Endpoints))
	}
}
