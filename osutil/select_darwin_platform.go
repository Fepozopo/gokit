package osutil

import "fmt"

func selectFileDarwin(title string) (string, error) {
	escaped := escapeAppleScriptString(title)
	script := fmt.Sprintf(`POSIX path of (choose file with prompt "%s")`, escaped)
	out, err := runCommandOutput("osascript", "-e", script)
	if err != nil {
		if isOsascriptCancel(err) {
			return "", nil
		}
		return "", fmt.Errorf("osascript error: %w", err)
	}
	return normalizeSingleSelection(string(out)), nil
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
	out, err := runCommandOutput("osascript", "-e", script)
	if err != nil {
		if isOsascriptCancel(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("osascript error: %w", err)
	}
	return parseSelectionList(string(out)), nil
}

func selectDirDarwin(title string) (string, error) {
	escaped := escapeAppleScriptString(title)
	script := fmt.Sprintf(`POSIX path of (choose folder with prompt "%s")`, escaped)
	out, err := runCommandOutput("osascript", "-e", script)
	if err != nil {
		if isOsascriptCancel(err) {
			return "", nil
		}
		return "", fmt.Errorf("osascript error: %w", err)
	}
	return normalizeSingleSelection(string(out)), nil
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
	out, err := runCommandOutput("osascript", "-e", script)
	if err != nil {
		if isOsascriptCancel(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("osascript error: %w", err)
	}
	return parseSelectionList(string(out)), nil
}
