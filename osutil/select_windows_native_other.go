//go:build !windows

package osutil

// nativeSelectFileWindows is the non-Windows stub for the native Windows
// single-file picker.
//
// The runtime dispatch in `select.go` still requires the symbol to exist on
// non-Windows hosts, but calling it directly outside Windows is unsupported.
func nativeSelectFileWindows(string) (string, error) {
	return "", ErrNoGUISelection
}

// nativeSelectFilesWindows is the non-Windows stub for the native Windows
// multi-file picker.
func nativeSelectFilesWindows(string) ([]string, error) {
	return nil, ErrNoGUISelection
}

// nativeSelectDirWindows is the non-Windows stub for the native Windows
// single-directory picker.
func nativeSelectDirWindows(string) (string, error) {
	return "", ErrNoGUISelection
}

// nativeSelectDirsWindows is the non-Windows stub for the native Windows
// multi-directory picker.
func nativeSelectDirsWindows(string) ([]string, error) {
	return nil, ErrNoGUISelection
}
