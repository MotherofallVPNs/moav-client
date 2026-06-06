// Package cmd implements CLI subcommands for moav-client.
package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ibeezhan/moav-client/proxy-core/prober"
	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// Version is set at build time.
const Version = "1.2.0"

// ParseAndRun parses os.Args and dispatches to the appropriate subcommand.
// Returns the subcommand name so main can decide whether to run the server.
// If no subcommand is given, it returns "serve".
func ParseAndRun(globalConfig *string) string {
	if len(os.Args) < 2 {
		return "serve"
	}

	switch os.Args[1] {
	case "serve":
		return "serve"
	case "version":
		fmt.Printf("moav-client %s\n", Version)
		os.Exit(0)
	case "probe":
		runProbe(globalConfig)
		os.Exit(0)
	case "list":
		runList(globalConfig)
		os.Exit(0)
	case "fetch-sub":
		runFetchSub()
		os.Exit(0)
	case "healthcheck":
		runHealthcheck()
		os.Exit(0)
	case "help", "--help", "-h":
		printUsage()
		os.Exit(0)
	default:
		// Unknown subcommand — fall through to serve (legacy behaviour).
		return "serve"
	}
	return "serve"
}

// runHealthcheck is the container healthcheck: the proxy-core image is built
// FROM scratch, so there's no wget/curl/sh to probe the API — the binary
// checks its own /api/healthz and exits 0 (healthy) or 1. Port defaults to
// 8088; override with MOAV_API_PORT if proxy.api_port is customized.
func runHealthcheck() {
	port := "8088"
	if v := os.Getenv("MOAV_API_PORT"); v != "" {
		port = v
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		os.Exit(1)
	}
	os.Exit(0)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `moav-client — MoaV proxy client with load-balancing and web dashboard

Usage:
  moav-client [command] [flags]

Commands:
  serve       Start the proxy + web UI (default if no command given)
  probe       Run a one-shot probe of all endpoints and print results
  list        List endpoints from subscription (no probing)
  fetch-sub   Fetch a subscription URL and print parsed endpoints
  version     Print version
  help        Print this help

Global flags:
  --config    Path to config.yaml (default: config.yaml)

probe flags:
  --timeout   Probe timeout in seconds (default: 10)
  --json      Output as JSON instead of table

list flags:
  --json      Output as JSON

fetch-sub positional argument:
  <url>       Subscription URL to fetch
`)
}

func loadEndpointsFromConfig(cfgPath string) ([]subscription.Endpoint, error) {
	// Inline the minimal logic needed (avoids importing the whole server stack).
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", cfgPath, err)
	}

	// Simple YAML extraction using the config package is not available here
	// without a circular dep risk, so we parse the subscription fields manually
	// by shelling out to the real config loader via the shared package.
	// Instead, re-use the subscription helpers directly.
	_ = data

	// Attempt to use the config package via the parent binary's init path.
	// For the CLI subcommands we accept the config path and re-use the same
	// logic as main.go but without starting network listeners.
	return nil, fmt.Errorf("use loadEndpointsViaConfig instead")
}

// endpointSource is passed in from main after config is loaded.
type EndpointSource struct {
	Endpoints []subscription.Endpoint
}

// RunProbeWithEndpoints runs probe using already-loaded endpoints.
func RunProbeWithEndpoints(endpoints []subscription.Endpoint, timeoutSec int, asJSON bool) {
	p := prober.New()
	p.Timeout = time.Duration(timeoutSec) * time.Second
	updated := p.ProbeAll(endpoints)
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(updated)
		return
	}
	printEndpointTable(updated)
}

// RunListWithEndpoints prints already-loaded endpoints without probing.
func RunListWithEndpoints(endpoints []subscription.Endpoint, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(endpoints)
		return
	}
	printEndpointTable(endpoints)
}

func printEndpointTable(eps []subscription.Endpoint) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROTOCOL\tADDRESS\tSTATUS\tLATENCY(ms)")
	fmt.Fprintln(w, "----\t--------\t-------\t------\t-----------")
	for _, ep := range eps {
		latency := fmt.Sprintf("%d", ep.LatencyMs)
		if ep.LatencyMs < 0 {
			latency = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			ep.Name, ep.Protocol, ep.Address, ep.Status, latency)
	}
	w.Flush()
}

// runProbe is called when os.Args[1] == "probe". globalConfig is the --config flag value.
func runProbe(globalConfig *string) {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	timeoutSec := fs.Int("timeout", 10, "probe timeout in seconds")
	asJSON := fs.Bool("json", false, "output as JSON")
	fs.Parse(os.Args[2:])

	eps := mustLoadEndpoints(*globalConfig)
	RunProbeWithEndpoints(eps, *timeoutSec, *asJSON)
}

// runList is called when os.Args[1] == "list".
func runList(globalConfig *string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "output as JSON")
	fs.Parse(os.Args[2:])

	eps := mustLoadEndpoints(*globalConfig)
	RunListWithEndpoints(eps, *asJSON)
}

// runFetchSub is called when os.Args[1] == "fetch-sub".
func runFetchSub() {
	fs := flag.NewFlagSet("fetch-sub", flag.ExitOnError)
	fs.Parse(os.Args[2:])

	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "fetch-sub: missing <url> argument")
		os.Exit(1)
	}

	eps, err := subscription.FetchSubscription(args[0], 30*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch-sub: %v\n", err)
		os.Exit(1)
	}
	printEndpointTable(eps)
}

// mustLoadEndpoints loads endpoints from a config file, printing errors and exiting on failure.
func mustLoadEndpoints(cfgPath string) []subscription.Endpoint {
	// Read the file to extract subscription.file and subscription.url fields.
	// We use a minimal inline YAML reader to avoid full config package dependency in this file.
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	// Extract subscription fields via simple line scan (avoids import cycle).
	subFile, subURL := extractSubscriptionFields(string(data))

	seen := make(map[string]struct{})
	var endpoints []subscription.Endpoint

	add := func(eps []subscription.Endpoint) {
		for _, ep := range eps {
			if _, dup := seen[ep.RawURI]; dup {
				continue
			}
			seen[ep.RawURI] = struct{}{}
			endpoints = append(endpoints, ep)
		}
	}

	if subFile != "" {
		raw, err := os.ReadFile(subFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "subscription file: %v\n", err)
		} else {
			eps, err := subscription.ParseSubscription(string(raw))
			if err != nil {
				fmt.Fprintf(os.Stderr, "subscription parse: %v\n", err)
			} else {
				add(eps)
			}
		}
	}

	if subURL != "" {
		eps, err := subscription.FetchSubscription(subURL, 30*time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "subscription fetch: %v\n", err)
		} else {
			add(eps)
		}
	}

	return endpoints
}

// extractSubscriptionFields does a minimal YAML scan for subscription.file and subscription.url.
func extractSubscriptionFields(yaml string) (file, url string) {
	inSubscription := false
	for _, line := range splitLines(yaml) {
		trimmed := trimSpace(line)
		if trimmed == "subscription:" {
			inSubscription = true
			continue
		}
		if inSubscription {
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				inSubscription = false
				continue
			}
			if k, v, ok := splitKV(trimmed); ok {
				switch k {
				case "file":
					file = v
				case "url":
					url = v
				}
			}
		}
	}
	return
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func splitKV(s string) (k, v string, ok bool) {
	for i, c := range s {
		if c == ':' {
			k = trimSpace(s[:i])
			v = trimSpace(s[i+1:])
			// Strip inline YAML quotes.
			if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
				v = v[1 : len(v)-1]
			}
			if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
				v = v[1 : len(v)-1]
			}
			return k, v, true
		}
	}
	return "", "", false
}
