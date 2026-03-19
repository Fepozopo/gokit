package utils

import (
	"fmt"
	"os"
	"strings"
)

// LoadDotEnv parses a simple .env file and sets environment variables.
// Supports comments (#), optional "export " prefix, and quoted values.
// Returns an error if the file cannot be read; callers may ignore the error
func LoadDotEnv(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		val = strings.ReplaceAll(val, `\n`, "\n")
		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf("setenv %q failed: %w", key, err)
		}
	}
	return nil
}
