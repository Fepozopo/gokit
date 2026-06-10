package osutil

// selectFileWindows opens a native Windows file picker and returns one file path.
func selectFileWindows(title string) (string, error) {
	selected, err := nativeSelectFileWindows(title)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(selected), nil
}

// selectFilesWindows opens a native Windows file picker and returns multiple file paths.
func selectFilesWindows(title string) ([]string, error) {
	selected, err := nativeSelectFilesWindows(title)
	if err != nil {
		return nil, err
	}
	return selected, nil
}

// selectDirWindows opens a native Windows folder picker and returns one directory path.
func selectDirWindows(title string) (string, error) {
	selected, err := nativeSelectDirWindows(title)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(selected), nil
}

// selectDirsWindows opens a native Windows picker and returns multiple directory paths.
func selectDirsWindows(title string) ([]string, error) {
	selected, err := nativeSelectDirsWindows(title)
	if err != nil {
		return nil, err
	}
	return selected, nil
}
