package api

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Byte-level and value-level pins for the comment-preserving config.yaml
// editor, so a YAML library swap that changes emission fails here first.

// canonicalYAML is already in the editor's output form (4-space indent), so a
// no-op edit must reproduce it byte for byte.
const canonicalYAML = `# moav-client config
# second header line

proxy:
    socks5_port: 1080 # overridden by .env
    http_port: 8080
    auth:
        username: "" # empty = no auth
        password: 'it''s: "quoted"'
# before subscription
subscription:
    url: "https://example.com/sub?a=1&b=2"
    wireguard_files: []
    sources:
        # first source
        - name: alpha # trailing
          file: ./alpha.txt
        # second source
        - name: "بتا"
          wireguard_files: [a.conf, b.conf]
plugins:
    torrent_block: false
    routing_rules:
        - match: {type: geoip, value: ir}
          action: direct
        - match:
            type: port
            value: "1000-2000"
          action: block
          enabled: false
          note: |
            two
            lines
sidecars:
    psiphon:
        enabled: yes
        config:
            config_json: |-
                {
                  "PropagationChannelId": "ABC"
                }
    masterdns:
        priority: 1
        config:
            method: "5"
            key: 00ff
# trailing comment
`

func readString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func noopEdit(t *testing.T, p string) string {
	t.Helper()
	if err := editYAMLFile(p, func(yamlMap) error { return nil }); err != nil {
		t.Fatalf("editYAMLFile: %v", err)
	}
	return readString(t, p)
}

func decodeAny(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode: %v\n%s", err, s)
	}
	return v
}

func TestEditYAML_NoopEditIsByteIdentical(t *testing.T) {
	p := writeTemp(t, canonicalYAML)
	if got := noopEdit(t, p); got != canonicalYAML {
		t.Errorf("no-op edit changed the file\n--- got ---\n%s\n--- want ---\n%s", got, canonicalYAML)
	}
}

func TestEditYAML_SurgicalEditTouchesOnlyTargetLines(t *testing.T) {
	p := writeTemp(t, canonicalYAML)
	err := editYAMLFile(p, func(root yamlMap) error {
		root.child("proxy").set("http_port", 9090)
		root.child("plugins").set("block_direct", true)
		root.child("sidecars").child("tor").set("enabled", true)
		if !root.child("subscription").del("url") {
			t.Error("del(url) should report the key existed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := canonicalYAML
	for _, r := range [][2]string{
		{"    http_port: 8080\n", "    http_port: 9090\n"},
		{"    url: \"https://example.com/sub?a=1&b=2\"\n", ""},
		{"            lines\nsidecars:", "            lines\n    block_direct: true\nsidecars:"},
		{"            key: 00ff\n", "            key: 00ff\n    tor:\n        enabled: true\n"},
	} {
		if !strings.Contains(want, r[0]) {
			t.Fatalf("fixture drifted: %q not found", r[0])
		}
		want = strings.Replace(want, r[0], r[1], 1)
	}
	if got := readString(t, p); got != want {
		t.Errorf("edit output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestEditYAML_ReindentsHandWrittenFileOnceThenStable(t *testing.T) {
	// A hand-written 2-space file is re-indented to 4 spaces on first write;
	// every later write must then be a fixed point.
	src := "# top\nproxy:\n  socks5_port: 1080   # aligned comment\n  auth:\n    username: \"\"\n\nplugins:\n  routing_rules:\n    - match: {type: geoip, value: ir}\n      action: direct\n"
	want := "# top\nproxy:\n    socks5_port: 1080 # aligned comment\n    auth:\n        username: \"\"\nplugins:\n    routing_rules:\n        - match: {type: geoip, value: ir}\n          action: direct\n"
	p := writeTemp(t, src)
	first := noopEdit(t, p)
	if first != want {
		t.Errorf("first write\n--- got ---\n%s\n--- want ---\n%s", first, want)
	}
	if second := noopEdit(t, p); second != first {
		t.Errorf("second write is not a fixed point\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

var commentRe = regexp.MustCompile(`#\s*(.*\S)\s*$`)

func TestEditYAML_ExampleConfigKeepsEveryCommentAndValue(t *testing.T) {
	src := readString(t, filepath.Join("..", "..", "config.yaml.example"))
	p := writeTemp(t, src)
	out := noopEdit(t, p)

	if !reflect.DeepEqual(decodeAny(t, out), decodeAny(t, src)) {
		t.Errorf("values changed across a no-op edit of config.yaml.example:\n%s", out)
	}
	for i, line := range strings.Split(src, "\n") {
		m := commentRe.FindStringSubmatch(line)
		if m == nil || strings.Contains(line, `"`) && !strings.HasPrefix(strings.TrimSpace(line), "#") && strings.Index(line, "#") < strings.LastIndex(line, `"`) {
			continue
		}
		if !strings.Contains(out, m[1]) {
			t.Errorf("config.yaml.example line %d: comment %q lost", i+1, m[1])
		}
	}
	if again := noopEdit(t, p); again != out {
		t.Error("second no-op edit of config.yaml.example is not a fixed point")
	}
}

func TestEditYAML_FailureLeavesFileUntouched(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		body    string
		fn      func(yamlMap) error
		wantErr func(error) bool
	}{
		{"callback error", canonicalYAML, func(root yamlMap) error {
			root.set("mutated", true)
			return boom
		}, func(err error) bool { return errors.Is(err, boom) }},
		{"skip write", canonicalYAML, func(root yamlMap) error {
			root.set("mutated", true)
			return errSkipWrite
		}, func(err error) bool { return err == nil }},
		{"malformed yaml", "proxy:\n  socks5_port: [1, 2\n", func(yamlMap) error {
			t.Error("callback must not run for a file that does not parse")
			return nil
		}, func(err error) bool { return err != nil && strings.Contains(err.Error(), "yaml:") }},
		{"tab indentation", "proxy:\n\tsocks5_port: 1\n", func(yamlMap) error {
			t.Error("callback must not run for a file that does not parse")
			return nil
		}, func(err error) bool { return err != nil && strings.Contains(err.Error(), "line 2") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeTemp(t, tc.body)
			err := editYAMLFile(p, tc.fn)
			if !tc.wantErr(err) {
				t.Errorf("unexpected error result: %v", err)
			}
			if got := readString(t, p); got != tc.body {
				t.Errorf("file was modified:\n%s", got)
			}
		})
	}

	if err := editYAMLFile(filepath.Join(t.TempDir(), "missing.yaml"), func(yamlMap) error { return nil }); !os.IsNotExist(err) {
		t.Errorf("missing file: want not-exist error, got %v", err)
	}
}

func TestEditYAML_EmptyAndCommentOnlyFiles(t *testing.T) {
	for name, body := range map[string]string{
		"empty":        "",
		"comment-only": "# nothing here yet\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := writeTemp(t, body)
			err := editYAMLFile(p, func(root yamlMap) error {
				root.child("plugins").set("block_direct", true)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			got := readString(t, p)
			want := map[string]any{"plugins": map[string]any{"block_direct": true}}
			if v := decodeAny(t, got); !reflect.DeepEqual(v, want) {
				t.Errorf("got %#v from:\n%s", v, got)
			}
		})
	}
}

func TestYAMLMap_Operations(t *testing.T) {
	p := writeTemp(t, "a: 1 # keep\nb:\n    c: x\nlist: [1, 2]\nscalar: v\n")
	err := editYAMLFile(p, func(root yamlMap) error {
		if root.get("missing") != nil {
			t.Error("get(missing) should be nil")
		}
		if got := root.scalarString("a"); got != "1" {
			t.Errorf("scalarString(a) = %q", got)
		}
		if got := root.scalarString("b"); got != "" {
			t.Errorf("scalarString of a mapping = %q, want empty", got)
		}
		if got := root.child("b").scalarString("c"); got != "x" {
			t.Errorf("child(b).c = %q", got)
		}
		if n := root.seq("list"); len(n.Content) != 2 {
			t.Errorf("seq(list) has %d items", len(n.Content))
		}
		if root.del("missing") {
			t.Error("del(missing) should be false")
		}
		// child() on a scalar replaces it with a mapping; seq() likewise.
		root.child("scalar").set("nested", "yes")
		root.seq("b")
		root.seq("fresh").Content = append(root.seq("fresh").Content, yamlNodeOf("item"))
		root.set("a", 2)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Replacing a value drops its line comment: the comment rides on the value node.
	want := "a: 2\nb: []\nlist: [1, 2]\nscalar:\n    nested: \"yes\"\nfresh:\n    - item\n"
	if got := readString(t, p); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestYAMLNodeOf_StringsSurviveARoundTripAsStrings(t *testing.T) {
	// Values the dashboard may write (passwords, SNI names, notes). Each must
	// come back as the identical Go string, never as a bool/number/null.
	values := []string{
		"", " ", "plain", "two words", " leading", "trailing ",
		"true", "false", "yes", "no", "on", "off", "Yes", "NO", "y", "n",
		"null", "~", "Null", "123", "0123", "0x1F", "0o17", "1.5", "1e3", ".inf", ".nan", "-1", "+1", "1_000",
		"2026-11-23", "2026-11-23T10:00:00Z", "12:30:00",
		"a: b", "a:b", "key:", "# not a comment", "a #b", "- item", "? q", "| pipe", "> gt",
		"*alias", "&anchor", "!tag", "!!str", "@at", "`tick", "%pct", "{a: 1}", "[a, b]", "a, b",
		"'single'", `"double"`, `back\slash`, "it's", `say "hi"`,
		"فارسی و نیم\u200cفاصله", "emoji 🐾", "tab\there", "nbsp x", "bell\a", "nul\x00byte",
		"line1\nline2", "line1\nline2\n", "trailing newline\n", "two trailing\n\n", "crlf\r\nline",
		"  indented first\nsecond", "a\n\n\nb", "ends with space \nnext",
		// Leading or bare newlines: gopkg.in/yaml.v3 emitted these as block
		// scalars that read back without the newline (go-yaml/yaml#755).
		"\n", "\n\n", "\nleading newline", "\n\nleading blank line\n", " \n", "\n ",
		strings.Repeat("long ", 60),
	}
	for _, v := range values {
		p := writeTemp(t, "# c\nkeep: 1\n")
		if err := editYAMLFile(p, func(root yamlMap) error {
			root.child("section").set("value", v)
			return nil
		}); err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		out := readString(t, p)
		var back struct {
			Section struct {
				Value any `yaml:"value"`
			} `yaml:"section"`
		}
		if err := yaml.Unmarshal([]byte(out), &back); err != nil {
			t.Errorf("%q: written file does not parse: %v\n%s", v, err, out)
			continue
		}
		if got, ok := back.Section.Value.(string); !ok || got != v {
			t.Errorf("%q round-tripped as %#v\n%s", v, back.Section.Value, out)
		}
		if !strings.HasPrefix(out, "# c\nkeep: 1\n") {
			t.Errorf("%q: untouched prefix changed:\n%s", v, out)
		}
	}
}

func TestYAMLNodeOf_TypedValuesGolden(t *testing.T) {
	p := writeTemp(t, "")
	err := editYAMLFile(p, func(root yamlMap) error {
		root.set("bool_true", true)
		root.set("bool_false", false)
		root.set("int", 12800)
		root.set("neg", -1)
		root.set("float", 1.5)
		root.set("nil", nil)
		root.set("str", "chrome")
		root.set("strs", []string{"a", "b c", "true"})
		root.set("empty_seq", []any{})
		root.set("empty_map", map[string]any{})
		root.set("multiline", "one\ntwo\n")
		// Map keys are emitted sorted, whatever the insertion order.
		root.set("rule", map[string]any{
			"note":    "n",
			"match":   map[string]any{"value": "443", "type": "port"},
			"enabled": false,
			"action":  "block",
		})
		root.set("rules", []map[string]any{{"b": 1, "a": "x"}, {"c": []string{"y"}}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `bool_true: true
bool_false: false
int: 12800
neg: -1
float: 1.5
nil: null
str: chrome
strs:
    - a
    - b c
    - "true"
empty_seq: []
empty_map: {}
multiline: |
    one
    two
rule:
    action: block
    enabled: false
    match:
        type: port
        value: "443"
    note: "n"
rules:
    - a: x
      b: 1
    - c:
        - "y"
`
	if got := readString(t, p); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEditYAML_AnchorsAndMergeKeysKeepTheirMeaning(t *testing.T) {
	src := "base: &base\n    enabled: true\n    priority: 5\nsidecars:\n    tor:\n        <<: *base\n        priority: 9\nfiles: &f [a.conf, b.conf]\nagain: *f\n"
	p := writeTemp(t, src)
	if err := editYAMLFile(p, func(root yamlMap) error {
		root.child("plugins").set("block_direct", true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out := readString(t, p)
	var back struct {
		Sidecars map[string]struct {
			Enabled  bool `yaml:"enabled"`
			Priority int  `yaml:"priority"`
		} `yaml:"sidecars"`
		Again []string `yaml:"again"`
	}
	if err := yaml.Unmarshal([]byte(out), &back); err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, out)
	}
	if tor := back.Sidecars["tor"]; !tor.Enabled || tor.Priority != 9 {
		t.Errorf("merge key lost its meaning: %+v\n%s", tor, out)
	}
	if !reflect.DeepEqual(back.Again, []string{"a.conf", "b.conf"}) {
		t.Errorf("alias lost: %v\n%s", back.Again, out)
	}
	for _, want := range []string{"&base", "*base", "&f [a.conf, b.conf]", "again: *f"} {
		if !strings.Contains(out, want) {
			t.Errorf("anchor/alias syntax %q not preserved:\n%s", want, out)
		}
	}
	// gopkg.in/yaml.v3 rewrote the merge key as "!!merge <<: *base" on every save.
	if want := src + "plugins:\n    block_direct: true\n"; out != want {
		t.Errorf("merge key not preserved verbatim\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestEditYAML_UnhashableMergeKeyIsAnErrorNotAPanic(t *testing.T) {
	// gopkg.in/yaml.v3 panicked decoding this into a map; the handlers that
	// read config.yaml generically must get an ordinary error instead.
	body := "{[a]: 1, <<: {[b]: 2}}\n"
	p := writeTemp(t, body)
	s, _ := newTestMux(t, p, "", nil)
	if err := s.persistBlockDirect(true); err == nil || !strings.Contains(err.Error(), "yaml:") {
		t.Errorf("want a yaml error, got %v", err)
	}
	if got := readString(t, p); got != body {
		t.Errorf("file was rewritten: %q", got)
	}
}
