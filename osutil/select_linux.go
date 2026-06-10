//go:build linux

package osutil

import "fmt"

// OpenFileSelection shows the Linux native file selection dialog and returns
// the selected file path.
func OpenFileSelection(title string) (string, error) {
	return selectFileLinux(title)
}

// OpenFilesSelection allows selecting multiple files using a Linux native
// dialog. If you only need a single file, call OpenFileSelection instead.
func OpenFilesSelection(title string) ([]string, error) {
	return selectFilesLinux(title)
}

// OpenDirSelection shows the Linux native directory selection dialog.
func OpenDirSelection(title string) (string, error) {
	return selectDirLinux(title)
}

// OpenDirsSelection shows the Linux native multiple-directory selection
// dialog. It returns nil if the dialog is cancelled.
func OpenDirsSelection(title string) ([]string, error) {
	return selectDirsLinux(title)
}

// selectFileLinux opens a native Linux file picker and returns one file path.
func selectFileLinux(title string) (string, error) {
	// Choose the helper at runtime so the package works with whatever desktop tool is installed.
	switch linuxSelectionBackend() {
	case "zenity":
		return pickFileZenity(title)
	case "kdialog":
		return pickFileKDialog(title)
	default:
		return "", ErrNoGUISelection
	}
}

// pickFileZenity opens a file picker through zenity.
func pickFileZenity(title string) (string, error) {
	raw, err := runCancelableCombinedCommand("zenity", "--file-selection", fmt.Sprintf("--title=%s", title))
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(raw), nil
}

// pickFileKDialog opens a file picker through kdialog.
func pickFileKDialog(title string) (string, error) {
	raw, err := runCancelableCombinedCommand("kdialog", "--getopenfilename", "", title)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(raw), nil
}

// selectFilesLinux opens a native Linux file picker and returns multiple file paths.
func selectFilesLinux(title string) ([]string, error) {
	// Choose the helper at runtime so the package works with whatever desktop tool is installed.
	switch linuxSelectionBackend() {
	case "zenity":
		// Ask zenity for newline-separated output; parseSelectionList still accepts pipes.
		raw, err := runCancelableCombinedCommand("zenity", "--file-selection", "--multiple", `--separator=\n`, fmt.Sprintf("--title=%s", title))
		if err != nil {
			return nil, err
		}
		return parseSelectionList(raw), nil
	case "kdialog":
		raw, err := runCancelableCombinedCommand("kdialog", "--getopenfilename", "", title, "--multiple", "--separate-output")
		if err != nil {
			return nil, err
		}
		return parseSelectionList(raw), nil
	default:
		return nil, ErrNoGUISelection
	}
}

// selectDirLinux opens a native Linux folder picker and returns one directory path.
func selectDirLinux(title string) (string, error) {
	// Choose the helper at runtime so the package works with whatever desktop tool is installed.
	switch linuxSelectionBackend() {
	case "zenity":
		raw, err := runCancelableCombinedCommand("zenity", "--file-selection", "--directory", fmt.Sprintf("--title=%s", title))
		if err != nil {
			return "", err
		}
		return normalizeSingleSelection(raw), nil
	case "kdialog":
		raw, err := runCancelableCombinedCommand("kdialog", "--getexistingdirectory", "")
		if err != nil {
			return "", err
		}
		return normalizeSingleSelection(raw), nil
	default:
		return "", ErrNoGUISelection
	}
}

// selectDirsLinux opens a native Linux folder picker and returns multiple directory paths.
func selectDirsLinux(title string) ([]string, error) {
	// Choose the helper at runtime so the package works with whatever desktop tool is installed.
	switch linuxSelectionBackend() {
	case "zenity":
		// Ask zenity for newline-separated output; parseSelectionList still accepts pipes.
		raw, err := runCancelableCombinedCommand("zenity", "--file-selection", "--directory", "--multiple", `--separator=\n`, fmt.Sprintf("--title=%s", title))
		if err != nil {
			return nil, err
		}
		return parseSelectionList(raw), nil
	case "kdialog":
		raw, err := runCancelableCombinedCommand("kdialog", "--getexistingdirectory", "", "--multiple", "--separate-output")
		if err != nil {
			return nil, err
		}
		return parseSelectionList(raw), nil
	default:
		return nil, ErrNoGUISelection
	}
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
