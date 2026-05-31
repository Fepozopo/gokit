package osutil

import "fmt"

func selectFileWindows(title string) (string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$ofd = New-Object System.Windows.Forms.OpenFileDialog;
$ofd.Title = '%s';
if ($ofd.ShowDialog() -eq 'OK') { Write-Output $ofd.FileName }`, escapedTitle)
	raw, err := runCancelableCombinedCommand("powershell", "-NoProfile", "-Command", ps)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(raw), nil
}

func selectFilesWindows(title string) ([]string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$ofd = New-Object System.Windows.Forms.OpenFileDialog;
$ofd.Title = '%s';
$ofd.Multiselect = $true;
if ($ofd.ShowDialog() -eq 'OK') {
	$ofd.FileNames -join "`+"`n"+`"
}`, escapedTitle)
	raw, err := runCancelableCombinedCommand("powershell", "-NoProfile", "-Command", ps)
	if err != nil {
		return nil, err
	}
	return parseSelectionList(raw), nil
}

func selectDirWindows(title string) (string, error) {
	escapedTitle := escapePowerShellSingleQuotes(title)
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$fd = New-Object System.Windows.Forms.FolderBrowserDialog;
$fd.Description = '%s';
if ($fd.ShowDialog() -eq 'OK') { Write-Output $fd.SelectedPath }`, escapedTitle)
	raw, err := runCancelableCombinedCommand("powershell", "-NoProfile", "-Command", ps)
	if err != nil {
		return "", err
	}
	return normalizeSingleSelection(raw), nil
}

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
	raw, err := runCancelableCombinedCommand("powershell", "-NoProfile", "-Command", ps)
	if err != nil {
		return nil, err
	}
	return parseSelectionList(raw), nil
}
