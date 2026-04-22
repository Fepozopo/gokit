package osutil

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ErrNoGUISelection indicates a GUI selection helper is unavailable.
// Exported so callers can detect this state.
var ErrNoGUISelection = errors.New("no GUI selection helper available")

// OpenFileSelection shows the system's native file selection dialog and
// returns the selected file path. It uses no external Go dependencies;
// instead it shells out to platform-provided helpers where possible.
//
// Behavior by platform:
//   - macOS (darwin): uses `osascript` to run an AppleScript `choose file` dialog.
//   - Windows (windows): uses PowerShell with System.Windows.Forms.OpenFileDialog.
//   - Linux (linux): tries `zenity`, then `kdialog`. If neither is available it
//     returns ErrNoGUISelection.
//
// The `title` argument is used as the dialog title (when supported). If the
// user cancels the dialog, an empty string and a nil error are returned.
//
// Note: this function deliberately avoids pulling in third-party packages.
// It requires the helper programs listed above to be present on the system to
// show a GUI dialog.
func OpenFileSelection(title string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return selectFileDarwin(title)
	case "windows":
		return selectFileWindows(title)
	case "linux":
		return selectFileLinux(title)
	default:
		// attempt a POSIX-ish approach via zenity/kdialog
		return selectFileLinux(title)
	}
}

func selectFileDarwin(title string) (string, error) {
	escaped := escapeAppleScriptString(title)
	script := fmt.Sprintf(`POSIX path of (choose file with prompt "%s")`, escaped)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
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
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ee.ExitCode() == 1 {
			return true
		}
	}
	return false
}

func escapeAppleScriptString(s string) string {
	// AppleScript strings are double-quoted. Escape backslashes and double quotes.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func selectFileWindows(title string) (string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$ofd = New-Object System.Windows.Forms.OpenFileDialog;
$ofd.Title = '%s';
if ($ofd.ShowDialog() -eq 'OK') { Write-Output $ofd.FileName }`, escapedTitle)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		// If user cancelled the dialog, PowerShell often returns no output.
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
	return strings.ReplaceAll(s, `'`, `''`)
}

func selectFileLinux(title string) (string, error) {
	if lookPath("zenity") {
		return pickFileZenity(title)
	}
	if lookPath("kdialog") {
		return pickFileKDialog(title)
	}
	return "", ErrNoGUISelection
}

func pickFileZenity(title string) (string, error) {
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

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// OpenFilesSelection allows selecting multiple files using a native dialog.
// If you only need a single file, call OpenFileSelection instead.
func OpenFilesSelection(title string) ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		return selectFilesDarwin(title)
	case "windows":
		return selectFilesWindows(title)
	case "linux":
		return selectFilesLinux(title)
	default:
		return selectFilesLinux(title)
	}
}

func selectFilesDarwin(title string) ([]string, error) {
	escaped := escapeAppleScriptString(title)
	script := fmt.Sprintf(`set chosen to (choose file with prompt "%s" with multiple selections allowed)
set outList to {}
if class of chosen is list then
	repeat with i in chosen
		set end of outList to POSIX path of i
	end repeat
else
	set end of outList to POSIX path of chosen
end if
set AppleScript's text item delimiters to "\n"
return outList as string`, escaped)

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		if isOsascriptCancel(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("osascript error: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, "\n")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts, nil
}

func selectFilesWindows(title string) ([]string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
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

func selectFilesLinux(title string) ([]string, error) {
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
	return nil, ErrNoGUISelection
}

// -------------------- Directory selection functions --------------------

// OpenDirSelection shows a dialog to pick a single directory.
// Behavior mirrors OpenFileSelection but for directories.
func OpenDirSelection(title string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return selectDirDarwin(title)
	case "windows":
		return selectdirWindows(title)
	case "linux":
		return selectDirLinux(title)
	default:
		return selectDirLinux(title)
	}
}

func selectDirDarwin(title string) (string, error) {
	escaped := escapeAppleScriptString(title)
	script := fmt.Sprintf(`POSIX path of (choose folder with prompt "%s")`, escaped)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
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

func selectdirWindows(title string) (string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$fd = New-Object System.Windows.Forms.FolderBrowserDialog;
$fd.Description = '%s';
if ($fd.ShowDialog() -eq 'OK') { Write-Output $fd.SelectedPath }`, escapedTitle)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
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

func selectDirLinux(title string) (string, error) {
	if lookPath("zenity") {
		cmd := exec.Command("zenity", "--file-selection", "--directory", fmt.Sprintf("--title=%s", title))
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err != nil {
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
	if lookPath("kdialog") {
		cmd := exec.Command("kdialog", "--getexistingdirectory", "")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err != nil {
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
	return "", ErrNoGUISelection
}

// OpenDirsSelection shows a dialog to pick multiple directories (where supported).
// Returns a slice of selected directory paths, or nil if cancelled.
func OpenDirsSelection(title string) ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		return selectDirsDarwin(title)
	case "windows":
		return selectDirsWindows(title)
	case "linux":
		return selectDirsLinux(title)
	default:
		return selectDirsLinux(title)
	}
}

func selectDirsDarwin(title string) ([]string, error) {
	escaped := escapeAppleScriptString(title)
	script := fmt.Sprintf(`set chosen to (choose folder with prompt "%s" with multiple selections allowed)
set outList to {}
if class of chosen is list then
	repeat with i in chosen
		set end of outList to POSIX path of i
	end repeat
else
	set end of outList to POSIX path of chosen
end if
set AppleScript's text item delimiters to "\n"
return outList as string`, escaped)

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		if isOsascriptCancel(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("osascript error: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, "\n")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts, nil
}

func selectDirsWindows(title string) ([]string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$ofd = New-Object System.Windows.Forms.OpenFileDialog;
$ofd.Title = '%s';
$ofd.ValidateNames = $false;
$ofd.CheckFileExists = $false;
$ofd.FileName = 'Select Folders';
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
			// User cancelled.
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

func selectDirsLinux(title string) ([]string, error) {
	if lookPath("zenity") {
		cmd := exec.Command("zenity", "--file-selection", "--directory", "--multiple", `--separator=\n`, fmt.Sprintf("--title=%s", title))
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
	if lookPath("kdialog") {
		cmd := exec.Command("kdialog", "--getexistingdirectory", "", "--multiple", "--separate-output")
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
	return nil, ErrNoGUISelection
}
