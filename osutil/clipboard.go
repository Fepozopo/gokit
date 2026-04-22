package osutil

import (
    "fmt"
    "os/exec"
    "runtime"
    "strings"
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

    // Use a Reader for stdin and let cmd.Run manage process start/wait/cleanup.
    // This avoids leaking the child process or pipe if a write fails.
    cmd.Stdin = strings.NewReader(text)
    return cmd.Run()
}
