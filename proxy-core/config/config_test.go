package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestEffectiveSources_LegacyOnly(t *testing.T) {
	s := SubscriptionConfig{
		File:           "./sub.txt",
		WireGuardFiles: []string{"./wg.conf"},
	}
	got := s.EffectiveSources()
	want := []SourceConfig{
		{Name: "default", File: "./sub.txt", WireGuardFiles: []string{"./wg.conf"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("legacy fields should produce a single 'default' source\nwant: %+v\n got: %+v", want, got)
	}
}

func TestEffectiveSources_ListOnly(t *testing.T) {
	s := SubscriptionConfig{
		Sources: []SourceConfig{
			{Name: "srv-A", File: "./a/sub.txt"},
			{Name: "srv-B", URL: "https://example.com/sub"},
		},
	}
	got := s.EffectiveSources()
	if len(got) != 2 {
		t.Fatalf("want 2 sources, got %d", len(got))
	}
	if got[0].Name != "srv-A" || got[1].Name != "srv-B" {
		t.Errorf("wrong order or names: %+v", got)
	}
}

func TestEffectiveSources_LegacyAndList(t *testing.T) {
	s := SubscriptionConfig{
		File:    "./default/sub.txt",
		Sources: []SourceConfig{{Name: "extra", File: "./extra/sub.txt"}},
	}
	got := s.EffectiveSources()
	if len(got) != 2 {
		t.Fatalf("want 2 sources (default + extra), got %d", len(got))
	}
	if got[0].Name != "default" {
		t.Errorf("legacy fields should produce 'default' source first, got %q", got[0].Name)
	}
	if got[1].Name != "extra" {
		t.Errorf("explicit sources should follow legacy default, got %q", got[1].Name)
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
proxy:
  socks5_port: 0  # explicit zero — should be overridden by defaults loader?
subscription:
  file: "./sub.txt"
`
	if err := writeAll(path, yaml); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Subscription.File != "./sub.txt" {
		t.Errorf("subscription.file not loaded: %q", cfg.Subscription.File)
	}
	if cfg.LoadBalancing.Strategy != "latency" {
		t.Errorf("strategy default not applied: %q", cfg.LoadBalancing.Strategy)
	}
	if cfg.Singbox.BasePort != 10800 {
		t.Errorf("singbox.base_port default not applied: %d", cfg.Singbox.BasePort)
	}
	if cfg.Xray.BasePort != 11800 {
		t.Errorf("xray.base_port default not applied: %d", cfg.Xray.BasePort)
	}
}

func writeAll(path, body string) error {
	// We import os in tests at the top? simpler: import here via stdlib in main file.
	// Workaround to keep this file minimal: deferred.
	return writeFile(path, body)
}
