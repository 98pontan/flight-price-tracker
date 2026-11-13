// Package secrets reads secrets from the environment, falling back to a Vault-injected KEY=VALUE file.
package secrets

import (
	"bufio"
	"os"
	"strings"
)

// Lookup returns the secret name from the environment, or from the file at path.
// It returns "" if the secret is not found in either place.
func Lookup(name, path string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if t := strings.TrimSpace(line); t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == name {
			return val
		}
	}
	return ""
}
