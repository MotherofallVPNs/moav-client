package main

import (
	"net"
	"testing"
)

// envPort precedence: .env file wins over process env, both win over the
// config.yaml fallback; malformed/out-of-range values are ignored.
func TestEnvPort(t *testing.T) {
	const key = "SOCKS5_PORT"
	cases := []struct {
		name string
		env  string // process env ("" = unset)
		file string // .env file value ("" = absent)
		want int
	}{
		{"fallback when nothing set", "", "", 1080},
		{"file overrides fallback", "", "9000", 9000},
		{"env overrides fallback", "2080", "", 2080},
		{"file overrides process env", "2080", "9000", 9000},
		{"malformed file ignored, env used", "2080", "notaport", 2080},
		{"out-of-range ignored", "", "70000", 1080},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(key, c.env) // envPort treats "" as unset
			envFile := map[string]string{}
			if c.file != "" {
				envFile[key] = c.file
			}
			if got := envPort(envFile, key, 1080); got != c.want {
				t.Errorf("envPort=%d, want %d", got, c.want)
			}
		})
	}
}

// go.mod pins `godebug multipathtcp=0`: the public exploit for kernel
// CVE-2026-98432 needs an MPTCP listener, which Go 1.24+ creates by default.
func TestListenersDoNotUseMPTCP(t *testing.T) {
	var lc net.ListenConfig
	if lc.MultipathTCP() {
		t.Fatal("MPTCP is on for listeners; keep `godebug multipathtcp=0` in go.mod")
	}
}
