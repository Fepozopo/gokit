package osutil

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// CopyTextToClipboard detects the OS and executes the native clipboard command
func CopyTextToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	case "linux":
		// Linux (requires xclip or wl-copy)
		// We try xclip first (X11), common on most distros
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			// Fallback for Wayland
			cmd = exec.Command("wl-copy")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip or wl-copy)")
		}
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	// Get the pipe to the command's standard input
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return err
	}

	// Write the text to the pipe and close it
	if _, err := io.WriteString(in, text); err != nil {
		return err
	}
	in.Close()

	// Wait for the command to exit
	return cmd.Wait()
}
