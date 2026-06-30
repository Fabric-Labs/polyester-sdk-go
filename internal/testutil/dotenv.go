package testutil

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

var dotenvOnce sync.Once

func loadDotEnv() {
	dotenvOnce.Do(func() {
		for _, path := range []string{".env", "../.env", "../../.env"} {
			if parseEnvFile(path) == nil {
				return
			}
		}
	})
}

func parseEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

// EnvTruthy reports whether an env var is set to a truthy value (1, true, yes).
func EnvTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
