package config

import "os"

// writeFile is a thin wrapper so tests don't have to import os.
func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}
