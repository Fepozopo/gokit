package osutil

import "fmt"

// selectFileWindows opens a native Windows file picker and returns one file path.
func selectFileWindows(title string) (string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$ofd = New-Object System.Windows.Forms.OpenFileDialog;
$ofd.Title = '%s';
if ($ofd.ShowDialog() -eq 'OK') { Write-Output $ofd.FileName }`, escapedTitle)
	// Shell out to PowerShell so the package can use Windows Forms without extra bindings.
	raw, err := runCancelableCombinedCommand("powershell", "-NoProfile", "-Command", ps)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(raw), nil
}

// selectFilesWindows opens a native Windows file picker and returns multiple file paths.
func selectFilesWindows(title string) ([]string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$ofd = New-Object System.Windows.Forms.OpenFileDialog;
$ofd.Title = '%s';
$ofd.Multiselect = $true;
if ($ofd.ShowDialog() -eq 'OK') {
	$ofd.FileNames -join "`+"`n"+`"
}`, escapedTitle)
	// Shell out to PowerShell so the package can use Windows Forms without extra bindings.
	raw, err := runCancelableCombinedCommand("powershell", "-NoProfile", "-Command", ps)
	if err != nil {
		return nil, err
	}
	return parseSelectionList(raw), nil
}

// selectDirWindows opens a native Windows folder picker and returns one directory path.
func selectDirWindows(title string) (string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$fd = New-Object System.Windows.Forms.FolderBrowserDialog;
$fd.Description = '%s';
if ($fd.ShowDialog() -eq 'OK') { Write-Output $fd.SelectedPath }`, escapedTitle)
	// Shell out to PowerShell so the package can use Windows Forms without extra bindings.
	raw, err := runCancelableCombinedCommand("powershell", "-NoProfile", "-Command", ps)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(raw), nil
}

// selectDirsWindows opens a native Windows picker and returns multiple directory paths.
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
	// FolderBrowserDialog does not support multiselect, so this uses the package's
	// existing OpenFileDialog-based workaround for choosing multiple directories.
	raw, err := runCancelableCombinedCommand("powershell", "-NoProfile", "-Command", ps)
	if err != nil {
		return nil, err
	}
	return parseSelectionList(raw), nil
}
