package osutil

import (
	"errors"
	"runtime"
)

// ErrNoGUISelection indicates a GUI selection helper is unavailable.
// Exported so callers can detect this state.
var ErrNoGUISelection = errors.New("no GUI selection helper available")

var currentGOOS = runtime.GOOS

// OpenFileSelection shows the system's native file selection dialog and
// returns the selected file path. It uses no external Go dependencies;
// instead it shells out to platform-provided helpers where possible.
func OpenFileSelection(title string) (string, error) {
	switch currentGOOS {
	case "darwin":
		return selectFileDarwin(title)
	case "windows":
		return selectFileWindows(title)
	case "linux":
		return selectFileLinux(title)
	default:
		return selectFileLinux(title)
	}
}

// OpenFilesSelection allows selecting multiple files using a native dialog.
// If you only need a single file, call OpenFileSelection instead.
func OpenFilesSelection(title string) ([]string, error) {
	switch currentGOOS {
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

// OpenDirSelection shows a dialog to pick a single directory.
// Behavior mirrors OpenFileSelection but for directories.
func OpenDirSelection(title string) (string, error) {
	switch currentGOOS {
	case "darwin":
		return selectDirDarwin(title)
	case "windows":
		return selectDirWindows(title)
	case "linux":
		return selectDirLinux(title)
	default:
		return selectDirLinux(title)
	}
}

// OpenDirsSelection shows a dialog to pick multiple directories (where supported).
// Returns a slice of selected directory paths, or nil if cancelled.
func OpenDirsSelection(title string) ([]string, error) {
	switch currentGOOS {
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
