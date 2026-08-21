// Package bundles handles MoaV bundle imports: zip files containing a
// per-user subscription.txt + the protocol .conf / config files the server
// emits (WireGuard/AmneziaWG as moav-<tag>-wg.conf / -awg.conf, TrustTunnel
// as trusttunnel.toml, MasterDNS as masterdns-client_config.toml). The
// dashboard uploads these via POST /api/bundles
// and we extract them into data/<name>/, then register a new source in
// config.yaml so the next proxy-core restart (or hot reload) picks them
// up alongside any existing sources.
package bundles

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Result describes what was extracted from a bundle import.
type Result struct {
	Name              string   `json:"name"`              // directory we extracted into (data/<name>/)
	Files             []string `json:"files"`             // relative paths of extracted files
	SubscriptionPath  string   `json:"subscription_path"` // first subscription.txt found, if any
	WireGuardConfPath string   `json:"wireguard_conf"`    // wireguard.conf path
	AmneziaWGConfPath string   `json:"amneziawg_conf"`    // amneziawg.conf path
	MasterDNSDomain   string   `json:"masterdns_domain"`  // parsed from masterdns-instructions.txt
	MasterDNSKey      string   `json:"masterdns_key"`     // parsed from masterdns-instructions.txt
	MasterDNSMethod   string   `json:"masterdns_method"`  // parsed from masterdns-instructions.txt
	TrustTunnelPath   string   `json:"trusttunnel_path"`  // trusttunnel.toml path
}

// Extract unpacks a zip into <baseDir>/<name>/ and reports what it found.
// name is sanitized to a safe directory component; if empty we derive one
// from the first subscription file path inside the archive.
func Extract(zipBytes []byte, baseDir, requestedName string) (*Result, error) {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("not a valid zip: %w", err)
	}

	name := sanitizeName(requestedName)
	if name == "" {
		// Derive from the first non-empty top-level directory in the archive
		// (most MoaV bundles look like "<name>/subscription.txt").
		for _, f := range r.File {
			if idx := strings.Index(f.Name, "/"); idx > 0 {
				name = sanitizeName(f.Name[:idx])
				if name != "" {
					break
				}
			}
		}
	}
	if name == "" {
		name = "imported-bundle"
	}

	dest := filepath.Join(baseDir, name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}

	res := &Result{Name: name}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// First defang: reject any entry name that's absolute or contains a
		// parent-directory component. We do this BEFORE stripping the
		// leading directory so a crafted "../escape.txt" entry can't survive
		// strip-then-look-safe.
		if filepath.IsAbs(f.Name) ||
			strings.HasPrefix(f.Name, "../") ||
			strings.HasPrefix(f.Name, "..\\") ||
			strings.Contains(f.Name, "/../") ||
			strings.Contains(f.Name, "\\..\\") {
			continue
		}

		// Strip the archive's leading directory (if any) so files land
		// directly under data/<name>/ instead of data/<name>/<name>/.
		relPath := f.Name
		if idx := strings.Index(relPath, "/"); idx >= 0 {
			relPath = relPath[idx+1:]
		}
		if relPath == "" {
			continue
		}
		// Belt + braces: clean and re-check.
		clean := filepath.Clean(relPath)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		outPath := filepath.Join(dest, clean)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		if err := os.WriteFile(outPath, body, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", outPath, err)
		}
		res.Files = append(res.Files, clean)

		// v2 server bundles name the WG/AWG confs moav-<server-tag>-wg.conf /
		// -awg.conf (older bundles used the bare wireguard.conf / amneziawg.conf).
		// Match by suffix so both keep working. Check -awg before -wg: the leading
		// dash means "-wg.conf" never matches an "...awg.conf" name, but ordering
		// it first keeps the intent obvious.
		base := strings.ToLower(filepath.Base(clean))
		switch {
		case base == "subscription.txt":
			res.SubscriptionPath = outPath
		case base == "amneziawg.conf" || strings.HasSuffix(base, "-awg.conf"):
			res.AmneziaWGConfPath = outPath
		case base == "wireguard.conf" || strings.HasSuffix(base, "-wg.conf"):
			res.WireGuardConfPath = outPath
		case base == "trusttunnel.toml":
			res.TrustTunnelPath = outPath
		case base == "masterdns-client_config.toml":
			// v2 format: a flat KEY = value file (MasterDnsVPN / MahsaNG).
			d, k, m := parseMasterDNSToml(string(body))
			res.MasterDNSDomain, res.MasterDNSKey, res.MasterDNSMethod = d, k, m
		case base == "masterdns-instructions.txt":
			d, k, m := parseMasterDNSInstructions(string(body))
			res.MasterDNSDomain, res.MasterDNSKey, res.MasterDNSMethod = d, k, m
		}
	}

	return res, nil
}

// nameOK keeps directory names alphanumeric + dash + underscore + dot.
var nameOK = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "/")
	s = nameOK.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// parseMasterDNSInstructions extracts the three values shown in the
// instructions file (Tunnel Domain, encryption method id, encryption key).
// The file format is a free-form .txt; we look for "Tunnel Domain:" /
// "encryption method" / "Encryption key" sections.
func parseMasterDNSInstructions(body string) (domain, key, method string) {
	lines := strings.Split(body, "\n")
	for i, raw := range lines {
		l := strings.TrimSpace(raw)
		switch {
		case strings.EqualFold(l, "# Tunnel Domain:") || strings.HasPrefix(l, "# Tunnel Domain:"):
			if j := i + 1; j < len(lines) {
				domain = firstWord(lines[j])
			}
		case strings.Contains(strings.ToLower(l), "data encryption method"),
			strings.Contains(strings.ToLower(l), "encryption method"):
			if j := i + 1; j < len(lines) {
				method = firstWord(lines[j])
			}
		case strings.Contains(strings.ToLower(l), "encryption key"):
			if j := i + 1; j < len(lines) {
				key = firstWord(lines[j])
			}
		}
	}
	return
}

// parseMasterDNSToml reads the v2 masterdns-client_config.toml: a flat
// KEY = value file (MasterDnsVPN / MahsaNG). DOMAINS may be a comma list;
// we take the first. Replaces the free-form instructions.txt of v1 bundles.
func parseMasterDNSToml(body string) (domain, key, method string) {
	for _, raw := range strings.Split(body, "\n") {
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		k = strings.ToUpper(strings.TrimSpace(k))
		v = tomlScalar(v)
		switch k {
		case "DOMAINS", "DOMAIN":
			domain = firstWord(strings.ReplaceAll(v, ",", " "))
		case "ENCRYPTION_KEY":
			key = v
		case "DATA_ENCRYPTION_METHOD", "ENCRYPTION_METHOD":
			method = firstWord(v)
		}
	}
	return
}

// tomlScalar normalizes a raw TOML RHS: strips an inline "# comment", unwraps a
// ["a","b"] array to space-separated elements, and removes surrounding quotes.
func tomlScalar(v string) string {
	v = strings.TrimSpace(v)
	// Inline comment (TOML #) — only when preceded by whitespace, so a '#'
	// inside a hex key or quoted value is left alone.
	if i := strings.IndexByte(v, '#'); i > 0 && (v[i-1] == ' ' || v[i-1] == '\t') {
		v = strings.TrimSpace(v[:i])
	}
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		v = v[1 : len(v)-1]
		v = strings.NewReplacer(`"`, " ", `'`, " ", ",", " ").Replace(v)
		v = strings.TrimSpace(v)
	}
	return strings.Trim(v, `"'`)
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t#"); i >= 0 {
		return s[:i]
	}
	return s
}
