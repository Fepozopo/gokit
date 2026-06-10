//go:build windows

package osutil

// OpenFileSelection shows the Windows native file selection dialog and returns
// the selected file path.
func OpenFileSelection(title string) (string, error) {
	selected, err := nativeSelectFileWindows(title)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(selected), nil
}

// OpenFilesSelection allows selecting multiple files using the Windows native
// dialog. If you only need a single file, call OpenFileSelection instead.
func OpenFilesSelection(title string) ([]string, error) {
	return nativeSelectFilesWindows(title)
}

// OpenDirSelection shows the Windows native directory selection dialog.
func OpenDirSelection(title string) (string, error) {
	selected, err := nativeSelectDirWindows(title)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(selected), nil
}

// OpenDirsSelection shows the Windows native multiple-directory selection
// dialog. It returns nil if the dialog is cancelled.
func OpenDirsSelection(title string) ([]string, error) {
	return nativeSelectDirsWindows(title)
}
