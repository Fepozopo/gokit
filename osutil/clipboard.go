package osutil

import (
	"fmt"
	"os/exec"
	"strings"
)

// CopyTextToClipboard detects the OS and executes the native clipboard command.
func CopyTextToClipboard(text string) error {
	cmd, err := clipboardCommand()
	if err != nil {
		return err
	}
	// Native clipboard tools read the payload from stdin.
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// clipboardCommand builds the native clipboard command for the current platform.
func clipboardCommand() (*exec.Cmd, error) {
	name, args, err := clipboardCommandSpec(currentGOOS)
	if err != nil {
		return nil, err
	}
	return commandExec(name, args...), nil
}

// clipboardCommandSpec returns the executable name and arguments for the given OS.
func clipboardCommandSpec(goos string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "pbcopy", nil, nil
	case "windows":
		return "clip", nil, nil
	case "linux":
		// Prefer xclip when available, then fall back to wl-copy.
		if hasCommand("xclip") {
			return "xclip", []string{"-selection", "clipboard"}, nil
		}
		if hasCommand("wl-copy") {
			return "wl-copy", nil, nil
		}
		return "", nil, fmt.Errorf("no clipboard utility found (install xclip or wl-copy)")
	default:
		return "", nil, fmt.Errorf("unsupported operating system: %s", goos)
	}
}
