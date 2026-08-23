package main

import (
	"os"
	"testing"
)

// envPort precedence: .env file wins over process env, both win over the
// config.yaml fallback; malformed/out-of-range values are ignored.
func TestEnvPort(t *testing.T) {
	const key = "SOCKS5_PORT"
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })

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
			if c.env == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, c.env)
			}
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
