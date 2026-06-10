package osutil

import (
	"fmt"
	"strings"
)

// runCancelableCombinedCommand executes a dialog helper and normalizes its output.
func runCancelableCombinedCommand(name string, args ...string) (string, error) {
	raw, err := runCommandCombined(name, args...)
	if err != nil {
		// GUI helpers commonly exit non-zero on cancel, often without emitting text.
		if strings.TrimSpace(raw) == "" {
			return "", nil
		}
		return "", fmt.Errorf("%s error: %w: %s", name, err, raw)
	}
	return strings.TrimSpace(raw), nil
}

// normalizeSingleSelection trims the output for helpers that return one path.
func normalizeSingleSelection(raw string) string {
	return strings.TrimSpace(raw)
}

// parseSelectionList parses helper output into a slice of selected paths.
func parseSelectionList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	separator := "\n"
	// Zenity commonly uses newlines, while some helpers and test doubles use pipes.
	if strings.Contains(raw, "|") && !strings.Contains(raw, "\n") {
		separator = "|"
	}

	parts := strings.Split(raw, separator)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
