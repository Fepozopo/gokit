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

func isOsascriptCancel(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 1
	}
	return false
}

func escapeAppleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func escapePowerShellSingleQuotes(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}

func hasCommand(name string) bool {
	_, err := lookPathExec(name)
	return err == nil
}

func runCommandOutput(name string, args ...string) ([]byte, error) {
	return commandExec(name, args...).Output()
}

func runCommandCombined(name string, args ...string) (string, error) {
	cmd := commandExec(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func runCancelableCombinedCommand(name string, args ...string) (string, error) {
	raw, err := runCommandCombined(name, args...)
	if err != nil {
		if strings.TrimSpace(raw) == "" {
			return "", nil
		}
		return "", fmt.Errorf("%s error: %w: %s", name, err, raw)
	}
	return strings.TrimSpace(raw), nil
}

func normalizeSingleSelection(raw string) string {
	return strings.TrimSpace(raw)
}

func parseSelectionList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	separator := "\n"
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

func linuxSelectionBackend() string {
	if hasCommand("zenity") {
		return "zenity"
	}
	if hasCommand("kdialog") {
		return "kdialog"
	}
	return ""
}
