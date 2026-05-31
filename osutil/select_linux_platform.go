package osutil

import "fmt"

func selectFileLinux(title string) (string, error) {
	switch linuxSelectionBackend() {
	case "zenity":
		return pickFileZenity(title)
	case "kdialog":
		return pickFileKDialog(title)
	default:
		return "", ErrNoGUISelection
	}
}

func pickFileZenity(title string) (string, error) {
	raw, err := runCancelableCombinedCommand("zenity", "--file-selection", fmt.Sprintf("--title=%s", title))
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(raw), nil
}

func pickFileKDialog(title string) (string, error) {
	raw, err := runCancelableCombinedCommand("kdialog", "--getopenfilename", "", title)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(raw), nil
}

func selectFilesLinux(title string) ([]string, error) {
	switch linuxSelectionBackend() {
	case "zenity":
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

func selectDirLinux(title string) (string, error) {
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

func selectDirsLinux(title string) ([]string, error) {
	switch linuxSelectionBackend() {
	case "zenity":
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
