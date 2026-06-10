//go:build !darwin && !linux && !windows

package osutil

// OpenFileSelection reports that no built-in GUI selection helper is available
// for the current platform.
func OpenFileSelection(string) (string, error) {
	return "", ErrNoGUISelection
}

// OpenFilesSelection reports that no built-in GUI selection helper is
// available for the current platform.
func OpenFilesSelection(string) ([]string, error) {
	return nil, ErrNoGUISelection
}

// OpenDirSelection reports that no built-in GUI selection helper is available
// for the current platform.
func OpenDirSelection(string) (string, error) {
	return "", ErrNoGUISelection
}

// OpenDirsSelection reports that no built-in GUI selection helper is available
// for the current platform.
func OpenDirsSelection(string) ([]string, error) {
	return nil, ErrNoGUISelection
}
