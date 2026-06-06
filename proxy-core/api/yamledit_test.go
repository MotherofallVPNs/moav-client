package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleYAML = `# top comment
plugins:
    # keep me
    block_direct: true
    routing_rules:
        - action: direct
          match:
            type: geoip
            value: ir
subscription:
    # sources live here
    sources:
        - name: alpha
        - name: beta
sni_spoof:
    enabled: false
    default_utls: chrome  # inline note
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEditYAML_PreservesComments(t *testing.T) {
	p := writeTemp(t, sampleYAML)
	err := editYAMLFile(p, func(root yamlMap) error {
		root.child("sni_spoof").set("enabled", true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(p)
	s := string(out)
	for _, want := range []string{"# top comment", "# keep me", "# sources live here", "inline note"} {
		if !strings.Contains(s, want) {
			t.Errorf("comment %q was dropped:\n%s", want, s)
		}
	}
	if !strings.Contains(s, "enabled: true") {
		t.Errorf("sni_spoof.enabled not updated:\n%s", s)
	}
}

func TestEditYAML_ReplaceSequence(t *testing.T) {
	p := writeTemp(t, sampleYAML)
	err := editYAMLFile(p, func(root yamlMap) error {
		root.child("plugins").setNode("routing_rules", yamlNodeOf([]map[string]any{
			{"action": "block", "enabled": false, "match": map[string]any{"type": "port", "value": "80"}},
		}))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(p)
	s := string(out)
	if !strings.Contains(s, "value: \"80\"") && !strings.Contains(s, "value: 80") {
		t.Errorf("routing_rules not replaced:\n%s", s)
	}
	if strings.Contains(s, "value: ir") {
		t.Errorf("old rule still present:\n%s", s)
	}
	if !strings.Contains(s, "# top comment") {
		t.Errorf("comment dropped on sequence replace:\n%s", s)
	}
}

func TestEditYAML_AppendAndFilterSources(t *testing.T) {
	p := writeTemp(t, sampleYAML)
	// append gamma
	if err := editYAMLFile(p, func(root yamlMap) error {
		srcs := root.child("subscription").seq("sources")
		srcs.Content = append(srcs.Content, yamlNodeOf(map[string]any{"name": "gamma"}))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// remove alpha
	if err := editYAMLFile(p, func(root yamlMap) error {
		srcs := root.child("subscription").seq("sources")
		kept := srcs.Content[:0:0]
		for _, item := range srcs.Content {
			if (yamlMap{item}).scalarString("name") == "alpha" {
				continue
			}
			kept = append(kept, item)
		}
		srcs.Content = kept
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(p)
	s := string(out)
	if strings.Contains(s, "alpha") {
		t.Errorf("alpha not removed:\n%s", s)
	}
	for _, want := range []string{"beta", "gamma", "# sources live here"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q present:\n%s", want, s)
		}
	}
}
