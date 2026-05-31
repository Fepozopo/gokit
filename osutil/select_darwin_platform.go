package osutil

import "fmt"

// selectFileDarwin opens a native macOS file picker and returns one file path.
func selectFileDarwin(title string) (string, error) {
	escaped := escapeAppleScriptString(title)
	script := fmt.Sprintf(`POSIX path of (choose file with prompt "%s")`, escaped)
	// Shell out to osascript so the package can use the native dialog without extra bindings.
	out, err := runCommandOutput("osascript", "-e", script)
	if err != nil {
		// osascript exits with status 1 when the user dismisses the dialog.
		if isOsascriptCancel(err) {
			return "", nil
		}
		return "", fmt.Errorf("osascript error: %w", err)
	}
	return normalizeSingleSelection(string(out)), nil
}

// selectFilesDarwin opens a native macOS file picker and returns multiple file paths.
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
	// Shell out to osascript so the package can use the native dialog without extra bindings.
	out, err := runCommandOutput("osascript", "-e", script)
	if err != nil {
		// osascript exits with status 1 when the user dismisses the dialog.
		if isOsascriptCancel(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("osascript error: %w", err)
	}
	return parseSelectionList(string(out)), nil
}

// selectDirDarwin opens a native macOS folder picker and returns one directory path.
func selectDirDarwin(title string) (string, error) {
	escaped := escapeAppleScriptString(title)
	script := fmt.Sprintf(`POSIX path of (choose folder with prompt "%s")`, escaped)
	// Shell out to osascript so the package can use the native dialog without extra bindings.
	out, err := runCommandOutput("osascript", "-e", script)
	if err != nil {
		// osascript exits with status 1 when the user dismisses the dialog.
		if isOsascriptCancel(err) {
			return "", nil
		}
		return "", fmt.Errorf("osascript error: %w", err)
	}
	return normalizeSingleSelection(string(out)), nil
}

// selectDirsDarwin opens a native macOS folder picker and returns multiple directory paths.
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
	// Shell out to osascript so the package can use the native dialog without extra bindings.
	out, err := runCommandOutput("osascript", "-e", script)
	if err != nil {
		// osascript exits with status 1 when the user dismisses the dialog.
		if isOsascriptCancel(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("osascript error: %w", err)
	}
	return parseSelectionList(string(out)), nil
}
