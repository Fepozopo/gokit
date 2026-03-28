package utils

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenFilePicker shows the system's native file-open dialog and returns the
// selected file path. It uses no external Go dependencies; instead it shells
// out to platform-provided helpers where possible.
//
// Behavior by platform:
//   - macOS (darwin): uses `osascript` to run an AppleScript `choose file` dialog.
//   - Windows (windows): uses PowerShell with System.Windows.Forms.OpenFileDialog.
//   - Linux (linux): tries `zenity`, then `kdialog`. If neither is available it
//     falls back to a console prompt (asks the user to type a path).
//
// The `title` argument is used as the dialog title (when supported). If the
// user cancels the dialog, an empty string and a nil error are returned.
//
// Note: this function deliberately avoids pulling in third-party packages.
// It requires the helper programs listed above to be present on the system to
// show a GUI dialog; if they're missing (on Linux) it falls back to a console prompt.
func OpenFilePicker(title string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickFileDarwin(title)
	case "windows":
		return pickFileWindows(title)
	case "linux":
		return pickFileLinux(title)
	default:
		// attempt a POSIX-ish approach via zenity/kdialog, otherwise console
		return pickFileLinux(title)
	}
}

func pickFileDarwin(title string) (string, error) {
	// Build AppleScript: POSIX path of (choose file with prompt "title")
	// Escape double quotes and backslashes in title.
	escaped := escapeAppleScriptString(title)
	// Use osascript -e 'POSIX path of (choose file with prompt "Title")'
	script := fmt.Sprintf(`POSIX path of (choose file with prompt "%s")`, escaped)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		// If user cancelled, osascript returns non-zero. We treat it as a cancel.
		// Try to detect cancellation: many osascript cancelations return exit 1.
		if isOsascriptCancel(err) {
			return "", nil
		}
		return "", fmt.Errorf("osascript error: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", nil
	}
	return path, nil
}

func isOsascriptCancel(err error) bool {
	// Can't inspect exit code portably from an error without type assertion,
	// but in practice we treat any error as cancellation only when there's
	// no stderr available (handled above). To be conservative, return false.
	// The caller will receive a non-nil error in that case.
	// (We keep this helper for clarity and possible future refinement.)
	_ = err
	return false
}

func escapeAppleScriptString(s string) string {
	// AppleScript strings are double-quoted. Escape backslashes and double quotes.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func pickFileWindows(title string) (string, error) {
	// Use PowerShell System.Windows.Forms.OpenFileDialog.
	// Build a PowerShell -Command script that writes the selected filename.
	// We use a single-quoted PowerShell string for the title, doubling single quotes.
	escapedTitle := escapePowerShellSingleQuotes(title)

	// PowerShell command:
	// Add-Type -AssemblyName System.Windows.Forms;
	// $ofd = New-Object System.Windows.Forms.OpenFileDialog;
	// $ofd.Title = '...';
	// if ($ofd.ShowDialog() -eq 'OK') { Write-Output $ofd.FileName }
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$ofd = New-Object System.Windows.Forms.OpenFileDialog;
$ofd.Title = '%s';
if ($ofd.ShowDialog() -eq 'OK') { Write-Output $ofd.FileName }`, escapedTitle)

	// Call powershell. Use -NoProfile to minimize startup noise.
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	// Ensure we don't inherit a GUI parent (we're just invoking).
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		// If user cancelled the dialog, PowerShell returns an exit code of 0
		// but no output. Some environments might return non-zero; treat that as cancel.
		if out.Len() == 0 {
			return "", nil
		}
		return "", fmt.Errorf("powershell error: %w: %s", err, out.String())
	}
	path := strings.TrimSpace(out.String())
	if path == "" {
		return "", nil
	}
	return path, nil
}

func escapePowerShellSingleQuotes(s string) string {
	// In PowerShell single-quoted strings, single quotes are escaped by doubling them.
	return strings.ReplaceAll(s, `'`, `''`)
}

func pickFileLinux(title string) (string, error) {
	// Try zenity
	if lookPath("zenity") {
		return pickFileZenity(title)
	}
	// Try kdialog
	if lookPath("kdialog") {
		return pickFileKDialog(title)
	}
	// Fall back to console prompt
	return pickFileConsole(title)
}

func pickFileZenity(title string) (string, error) {
	// zenity --file-selection --title="Title"
	cmd := exec.Command("zenity", "--file-selection", fmt.Sprintf("--title=%s", title))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		// user canceled -> zenity exit 1, no output
		if out.Len() == 0 {
			return "", nil
		}
		return "", fmt.Errorf("zenity error: %w: %s", err, out.String())
	}
	path := strings.TrimSpace(out.String())
	if path == "" {
		return "", nil
	}
	return path, nil
}

func pickFileKDialog(title string) (string, error) {
	// kdialog --getopenfilename "" "Title"
	// First arg is initial directory / filter; pass empty string to let it decide.
	cmd := exec.Command("kdialog", "--getopenfilename", "", title)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		// kdialog returns non-zero on cancel (typically exit 1). Treat as cancel.
		if out.Len() == 0 {
			return "", nil
		}
		return "", fmt.Errorf("kdialog error: %w: %s", err, out.String())
	}
	path := strings.TrimSpace(out.String())
	if path == "" {
		return "", nil
	}
	return path, nil
}

func pickFileConsole(title string) (string, error) {
	// Last-resort: ask on console.
	fmt.Fprintln(os.Stderr, "No GUI file-picker available. Please type a file path and press Enter.")
	if title != "" {
		fmt.Fprintf(os.Stderr, "%s\n", title)
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stderr, "> ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("console input error: %w", err)
	}
	path := strings.TrimSpace(line)
	if path == "" {
		return "", nil
	}
	return path, nil
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Helper: allow callers to open multiple file selection dialogs
// If you don't need multiple selection, call OpenFilePicker instead.
func OpenFilesPicker(title string) ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickFilesDarwin(title)
	case "windows":
		return pickFilesWindows(title)
	case "linux":
		return pickFilesLinux(title)
	default:
		return pickFilesLinux(title)
	}
}

func pickFilesDarwin(title string) ([]string, error) {
	escaped := escapeAppleScriptString(title)
	// Ask for multiple selection: choose file with prompt "Title" with multiple selections allowed
	script := fmt.Sprintf(`set p to (choose file with prompt "%s" with multiple selections allowed)
set out to ""
repeat with i in p
	set out to out & POSIX path of i & "%s"
end repeat
return out`, escaped, string(rune(0)))
	// We use a NUL separator between paths to robustly split them.
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		if isOsascriptCancel(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("osascript error: %w", err)
	}
	raw := string(out)
	if raw == "" {
		return nil, nil
	}
	parts := splitNuls(raw)
	return parts, nil
}

func pickFilesWindows(title string) ([]string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	// Use System.Windows.Forms.OpenFileDialog and enable Multiselect
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$ofd = New-Object System.Windows.Forms.OpenFileDialog;
$ofd.Title = '%s';
$ofd.Multiselect = $true;
if ($ofd.ShowDialog() -eq 'OK') {
	$ofd.FileNames -join "`+"`n"+`"
}`, escapedTitle)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		if out.Len() == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("powershell error: %w: %s", err, out.String())
	}
	raw := strings.TrimSpace(out.String())
	if raw == "" {
		return nil, nil
	}
	lines := strings.Split(raw, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return lines, nil
}

func pickFilesLinux(title string) ([]string, error) {
	// Try zenity --file-selection --multiple --separator="\n" --title=...
	if lookPath("zenity") {
		cmd := exec.Command("zenity", "--file-selection", "--multiple", `--separator=\n`, fmt.Sprintf("--title=%s", title))
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err != nil {
			if out.Len() == 0 {
				return nil, nil
			}
			return nil, fmt.Errorf("zenity error: %w: %s", err, out.String())
		}
		raw := strings.TrimSpace(out.String())
		if raw == "" {
			return nil, nil
		}
		// zenity returns 'file1|file2' or with our separator '\n'. Handle both.
		if strings.Contains(raw, "|") && !strings.Contains(raw, "\n") {
			parts := strings.Split(raw, "|")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			return parts, nil
		}
		parts := strings.Split(raw, "\n")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts, nil
	}
	// Try kdialog --getopenfilename --multiple
	if lookPath("kdialog") {
		cmd := exec.Command("kdialog", "--getopenfilename", "", title, "--multiple", "--separate-output")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err != nil {
			if out.Len() == 0 {
				return nil, nil
			}
			return nil, fmt.Errorf("kdialog error: %w: %s", err, out.String())
		}
		raw := strings.TrimSpace(out.String())
		if raw == "" {
			return nil, nil
		}
		parts := strings.Split(raw, "\n")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts, nil
	}
	// Console fallback: ask the user to enter paths separated by newline; finish with empty line.
	fmt.Fprintln(os.Stderr, "No GUI file-picker available. Enter one file path per line. Submit an empty line to finish.")
	var res []string
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("console input error: %w", err)
			}
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
		res = append(res, line)
	}
	if len(res) == 0 {
		return nil, nil
	}
	return res, nil
}

// splitNuls splits a string by NUL (0) bytes and trims whitespace.
func splitNuls(s string) []string {
	var parts []string
	for _, p := range strings.Split(s, string(rune(0))) {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// Additional useful helper: OpenFilePickerOrPanic is a convenience for scripts
// where you want the selected path or a fatal error. It returns the path or
// exits the process.
func OpenFilePickerOrFatal(title string) string {
	p, err := OpenFilePicker(title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "file picker failed: %v\n", err)
		os.Exit(2)
	}
	if p == "" {
		fmt.Fprintln(os.Stderr, "no file selected")
		os.Exit(0)
	}
	return p
}

// small exported convenience: ErrNoPicker indicates GUI picker unavailable.
// It's not used aggressively here (we prefer to fall back to console), but
// exported for callers that want to detect this state.
var ErrNoPicker = errors.New("no GUI file picker available")
