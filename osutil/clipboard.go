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
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func clipboardCommand() (*exec.Cmd, error) {
	name, args, err := clipboardCommandSpec(currentGOOS)
	if err != nil {
		return nil, err
	}
	return commandExec(name, args...), nil
}

func clipboardCommandSpec(goos string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "pbcopy", nil, nil
	case "windows":
		return "clip", nil, nil
	case "linux":
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
