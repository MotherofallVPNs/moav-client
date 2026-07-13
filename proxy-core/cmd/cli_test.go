package cmd

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"no_ws", "abc", "abc"},
		{"leading_spaces", "   abc", "abc"},
		{"trailing_spaces", "abc   ", "abc"},
		{"both", "  abc  ", "abc"},
		{"tabs", "\t\tabc\t", "abc"},
		{"mixed_tabs_spaces", " \t abc \t ", "abc"},
		{"all_whitespace", "   \t  ", ""},
		{"inner_ws_preserved", "  a b c  ", "a b c"},
		// Note: trimSpace only trims spaces and tabs, not newlines/CR.
		{"newline_not_trimmed", "\nabc\n", "\nabc\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimSpace(tt.in); got != tt.want {
				t.Errorf("trimSpace(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitKV(t *testing.T) {
	tests := []struct {
		name, in     string
		wantK, wantV string
		wantOK       bool
	}{
		{"simple", "file: sub.txt", "file", "sub.txt", true},
		{"no_space", "file:sub.txt", "file", "sub.txt", true},
		{"double_quoted", `url: "https://x.example/sub"`, "url", "https://x.example/sub", true},
		{"single_quoted", "url: 'https://x.example/sub'", "url", "https://x.example/sub", true},
		{"no_colon", "just a value", "", "", false},
		{"empty", "", "", "", false},
		{"empty_value", "file:", "file", "", true},
		{"extra_whitespace", "  file  :  sub.txt  ", "file", "sub.txt", true},
		// The value after the FIRST colon includes any later colons (e.g. URLs).
		{"value_has_colon", "url: http://a:8080/x", "url", "http://a:8080/x", true},
		// A bare quote (only one) is not stripped.
		{"unbalanced_quote", `file: "half`, "file", `"half`, true},
		// Single char values can't be quote-stripped (needs len >= 2).
		{"single_char", "file: x", "file", "x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, v, ok := splitKV(tt.in)
			if ok != tt.wantOK || k != tt.wantK || v != tt.wantV {
				t.Errorf("splitKV(%q) = (%q,%q,%v), want (%q,%q,%v)",
					tt.in, k, v, ok, tt.wantK, tt.wantV, tt.wantOK)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"empty", "", nil},
		{"single_no_newline", "abc", []string{"abc"}},
		{"two_lines", "a\nb", []string{"a", "b"}},
		{"trailing_newline", "a\nb\n", []string{"a", "b"}},
		{"blank_lines", "a\n\nb", []string{"a", "", "b"}},
		{"only_newline", "\n", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitLines(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitLines(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractSubscriptionFields(t *testing.T) {
	tests := []struct {
		name              string
		yaml              string
		wantFile, wantURL string
	}{
		{
			name: "both_fields",
			yaml: "subscription:\n  file: data/sub.txt\n  url: https://x.example/sub\n",
			wantFile: "data/sub.txt", wantURL: "https://x.example/sub",
		},
		{
			name:     "only_file",
			yaml:     "subscription:\n  file: data/sub.txt\n",
			wantFile: "data/sub.txt",
		},
		{
			name:    "only_url_quoted",
			yaml:    "subscription:\n  url: \"https://x.example/sub\"\n",
			wantURL: "https://x.example/sub",
		},
		{
			name:     "block_boundary_ends_at_dedent",
			// The `proxy:` block is a sibling; its file/url must be ignored.
			yaml:     "subscription:\n  file: real.txt\nproxy:\n  file: wrong.txt\n  url: http://wrong\n",
			wantFile: "real.txt",
		},
		{
			name:     "resumes_only_inside_block",
			// Keys before the subscription: header are not in-block.
			yaml:     "file: ignored.txt\nsubscription:\n  file: kept.txt\n",
			wantFile: "kept.txt",
		},
		{
			name: "no_subscription_block",
			yaml: "proxy:\n  socks5_port: 1080\n",
		},
		{
			name:     "empty_input",
			yaml:     "",
		},
		{
			name:     "tab_indented",
			yaml:     "subscription:\n\tfile: tabbed.txt\n",
			wantFile: "tabbed.txt",
		},
		{
			name:     "later_block_reentry_ignored",
			// After dedent out of subscription: a second subscription: header
			// re-enters the block.
			yaml:     "subscription:\n  file: first.txt\nother:\n  x: y\nsubscription:\n  url: http://second\n",
			wantFile: "first.txt", wantURL: "http://second",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, url := extractSubscriptionFields(tt.yaml)
			if file != tt.wantFile || url != tt.wantURL {
				t.Errorf("extractSubscriptionFields(%q) = (file=%q, url=%q), want (file=%q, url=%q)",
					tt.yaml, file, url, tt.wantFile, tt.wantURL)
			}
		})
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what
// was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

func TestRunListWithEndpoints_JSON(t *testing.T) {
	eps := []subscription.Endpoint{
		{ID: "a", Protocol: "vless", Name: "n1", Address: "1.2.3.4:443", Status: "ok", LatencyMs: 10},
		{ID: "b", Protocol: "sidecar", Name: "n2", Address: "tor:9150", Status: "unknown", LatencyMs: -1},
	}
	out := captureStdout(t, func() {
		RunListWithEndpoints(eps, true)
	})

	var got []subscription.Endpoint
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 endpoints, got %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("endpoint order/content mismatch: %+v", got)
	}
}

func TestRunListWithEndpoints_Table(t *testing.T) {
	eps := []subscription.Endpoint{
		{ID: "a", Protocol: "vless", Name: "n1", Address: "1.2.3.4:443", Status: "ok", LatencyMs: 10},
		{ID: "b", Protocol: "sidecar", Name: "n2", Address: "tor:9150", Status: "unknown", LatencyMs: -1},
	}
	out := captureStdout(t, func() {
		RunListWithEndpoints(eps, false)
	})

	for _, want := range []string{"NAME", "PROTOCOL", "n1", "n2", "1.2.3.4:443", "tor:9150"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\noutput:\n%s", want, out)
		}
	}
	// A negative latency should render as "-", not "-1".
	if strings.Contains(out, "-1") {
		t.Errorf("negative latency should render as '-', got:\n%s", out)
	}
}
