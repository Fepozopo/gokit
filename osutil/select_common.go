package osutil

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var commandExec = exec.Command
var lookPathExec = exec.LookPath

// isOsascriptCancel reports whether err represents a user-cancelled osascript dialog.
func isOsascriptCancel(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 1
	}
	return false
}

// escapeAppleScriptString escapes s for embedding in a double-quoted AppleScript string.
func escapeAppleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// hasCommand reports whether name can be found in PATH.
func hasCommand(name string) bool {
	_, err := lookPathExec(name)
	return err == nil
}

// runCommandOutput executes name with args and returns stdout.
func runCommandOutput(name string, args ...string) ([]byte, error) {
	return commandExec(name, args...).Output()
}

// runCommandCombined executes name with args and returns combined stdout and stderr.
func runCommandCombined(name string, args ...string) (string, error) {
	cmd := commandExec(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

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

// linuxSelectionBackend reports the preferred Linux dialog helper available in PATH.
func linuxSelectionBackend() string {
	// Prefer zenity first, then fall back to kdialog.
	if hasCommand("zenity") {
		return "zenity"
	}
	if hasCommand("kdialog") {
		return "kdialog"
	}
	return ""
}
