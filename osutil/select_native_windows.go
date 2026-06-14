//go:build windows

package osutil

import (
	"errors"
	"fmt"
	"syscall"
)

// This file contains the high-level Windows picker flow for osutil selection
// helpers. Lower-level COM definitions, COM lifecycle helpers, and dialog
// interface wrappers live in the companion select_native_*_windows.go files.

// errNativeDialogCancelled marks a native Windows dialog dismissal without a
// selection.
//
// The public `osutil` selection helpers intentionally normalize cancellation to
// an empty result with a nil error, but the Windows COM layer still needs a
// precise internal signal so cancellation can be distinguished from real COM
// failures while the dialog flow unwinds.
var errNativeDialogCancelled = errors.New("cancelled")

// Windows picker entrypoints.

// nativeSelectFileWindows opens the Windows Common Item Dialog in single-file
// mode and normalizes user cancellation to the `osutil` convention of an empty
// result with a nil error.
func nativeSelectFileWindows(title string) (string, error) {
	selected, err := pickSingleWindowsPath(title, fosForceFilesystem|fosNoChangeDir|fosPathMustExist|fosFileMustExist, true)
	if errors.Is(err, errNativeDialogCancelled) {
		return "", nil
	}
	return selected, err
}

// nativeSelectFilesWindows opens the Windows Common Item Dialog in multi-file
// mode and returns every selected filesystem path.
//
// Like the other selection helpers in this package, cancellation is surfaced as
// a nil result and a nil error.
func nativeSelectFilesWindows(title string) ([]string, error) {
	selected, err := pickMultipleWindowsPaths(title, fosForceFilesystem|fosNoChangeDir|fosPathMustExist|fosFileMustExist|fosAllowMultiSelect, true)
	if errors.Is(err, errNativeDialogCancelled) {
		return nil, nil
	}
	return selected, err
}

// nativeSelectDirWindows opens the Windows Common Item Dialog in single-folder
// mode and returns the chosen directory path.
//
// Compared with the older folder browser APIs, this produces the modern
// Explorer-style folder picker without spawning PowerShell.
func nativeSelectDirWindows(title string) (string, error) {
	selected, err := pickSingleWindowsPath(title, fosPickFolders|fosForceFilesystem|fosNoChangeDir|fosPathMustExist, false)
	if errors.Is(err, errNativeDialogCancelled) {
		return "", nil
	}
	return selected, err
}

// nativeSelectDirsWindows opens the Windows Common Item Dialog in multi-folder
// mode and returns all selected directories.
//
// Windows exposes multi-select results through `IShellItemArray`, which this
// helper resolves into ordinary filesystem paths for the rest of the package.
func nativeSelectDirsWindows(title string) ([]string, error) {
	selected, err := pickMultipleWindowsPaths(title, fosPickFolders|fosForceFilesystem|fosNoChangeDir|fosPathMustExist|fosAllowMultiSelect, false)
	if errors.Is(err, errNativeDialogCancelled) {
		return nil, nil
	}
	return selected, err
}

// pickSingleWindowsPath configures and shows a Common Item Dialog that should
// resolve to exactly one filesystem path.
//
// File-selection callers can request default file filters, while folder pickers
// skip that configuration entirely.
func pickSingleWindowsPath(title string, options uint32, configureFileFilters bool) (string, error) {
	dialog, cleanup, err := createConfiguredFileOpenDialog(title, options)
	if err != nil {
		return "", err
	}
	defer cleanup()

	if configureFileFilters {
		if err := applyDefaultFileFilters(dialog); err != nil {
			return "", err
		}
	}

	return showDialogAndResolvePath(dialog)
}

// pickMultipleWindowsPaths configures and shows a Common Item Dialog that may
// return multiple filesystem paths.
//
// The dialog is still the same `IFileOpenDialog` object; the difference is the
// option flags and the use of `GetResults` instead of `GetResult` after the user
// confirms their selection.
func pickMultipleWindowsPaths(title string, options uint32, configureFileFilters bool) ([]string, error) {
	dialog, cleanup, err := createConfiguredFileOpenDialog(title, options)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if configureFileFilters {
		if err := applyDefaultFileFilters(dialog); err != nil {
			return nil, err
		}
	}

	return showDialogAndResolvePaths(dialog)
}

// applyDefaultFileFilters configures the file dialog with a generic all-files
// filter.
//
// This library does not currently expose filter selection in its public API, so
// the Windows backend keeps the configuration intentionally minimal while still
// ensuring the dialog stays in a normal file-picking mode.
func applyDefaultFileFilters(dialog *fileOpenDialog) error {
	filters, err := prepareDialogFilters([]fileDialogFilter{
		{DisplayName: "All files (*.*)", Pattern: "*.*"},
	})
	if err != nil {
		return err
	}
	if err := dialog.SetFileTypes(filters.Specs); err != nil {
		return err
	}
	if err := dialog.SetFileTypeIndex(1); err != nil {
		return err
	}
	return nil
}

// Dialog setup and lifecycle helpers.

// createConfiguredFileOpenDialog initializes COM, constructs an
// `IFileOpenDialog`, merges the requested option flags with the dialog's
// existing defaults, and applies the provided title.
//
// The returned cleanup function must be called exactly once. It releases the
// dialog first and then unwinds any COM initialization performed for the current
// thread.
func createConfiguredFileOpenDialog(title string, requiredOptions uint32) (*fileOpenDialog, func(), error) {
	cleanupCOM, err := initializeCOM()
	if err != nil {
		return nil, nil, err
	}

	dialog, err := createFileOpenDialog()
	if err != nil {
		if cleanupCOM != nil {
			cleanupCOM()
		}
		return nil, nil, err
	}

	cleanup := func() {
		dialog.Release()
		if cleanupCOM != nil {
			cleanupCOM()
		}
	}

	options, err := dialog.Options()
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	// Windows exposes some default flags through GetOptions. Start from that
	// baseline and add only the behavior this package requires so we do not
	// accidentally discard shell-provided defaults.
	options |= requiredOptions
	if err := dialog.SetOptions(options); err != nil {
		cleanup()
		return nil, nil, err
	}

	titleUTF16, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to encode dialog title: %w", err)
	}
	if err := dialog.SetTitle(titleUTF16); err != nil {
		cleanup()
		return nil, nil, err
	}

	return dialog, cleanup, nil
}

// showDialogAndResolvePath displays the configured dialog and returns the
// filesystem path chosen by the user.
//
// The helper centralizes the common cancellation handling and shell item
// resolution shared by the single file and single folder pickers.
func showDialogAndResolvePath(dialog *fileOpenDialog) (string, error) {
	// Pass a nil owner window because this package does not expose a stable HWND.
	// Windows still shows a normal modal picker.
	if err := dialog.Show(0); err != nil {
		var hrErr hresultError
		if errors.As(err, &hrErr) && hrErr.Code == hresultCanceled {
			return "", errNativeDialogCancelled
		}
		return "", err
	}

	// Once the dialog closes successfully, Windows exposes the selection as an
	// `IShellItem` that can be resolved into a filesystem path.
	item, err := dialog.Result()
	if err != nil {
		return "", err
	}
	defer item.Release()

	selected, err := item.DisplayName(sigdnFileSysPath)
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", errNativeDialogCancelled
	}
	return selected, nil
}

// showDialogAndResolvePaths displays the configured dialog and resolves every
// selected filesystem path from the returned `IShellItemArray`.
//
// This is the multi-select analogue to `showDialogAndResolvePath` and is shared
// by both file and directory multi-selection.
func showDialogAndResolvePaths(dialog *fileOpenDialog) ([]string, error) {
	if err := dialog.Show(0); err != nil {
		var hrErr hresultError
		if errors.As(err, &hrErr) && hrErr.Code == hresultCanceled {
			return nil, errNativeDialogCancelled
		}
		return nil, err
	}

	items, err := dialog.Results()
	if err != nil {
		return nil, err
	}
	defer items.Release()

	selected, err := items.DisplayNames(sigdnFileSysPath)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, errNativeDialogCancelled
	}
	return selected, nil
}

// prepareDialogFilters converts human-readable filter definitions into the UTF-16
// structures required by the Windows Common Item Dialog.
//
// The returned value intentionally retains the UTF-16 backing slices so the raw
// pointers embedded in `Specs` remain valid for the duration of the COM call.
func prepareDialogFilters(filters []fileDialogFilter) (*preparedDialogFilters, error) {
	if len(filters) == 0 {
		return nil, fmt.Errorf("at least one dialog filter is required")
	}

	prepared := &preparedDialogFilters{
		Specs:    make([]commonDialogFilterSpec, 0, len(filters)),
		Names:    make([][]uint16, 0, len(filters)),
		Patterns: make([][]uint16, 0, len(filters)),
	}

	for _, filter := range filters {
		if filter.DisplayName == "" {
			return nil, fmt.Errorf("dialog filter display name cannot be empty")
		}
		if filter.Pattern == "" {
			return nil, fmt.Errorf("dialog filter pattern cannot be empty")
		}

		name, err := syscall.UTF16FromString(filter.DisplayName)
		if err != nil {
			return nil, fmt.Errorf("invalid dialog filter display name %q: %w", filter.DisplayName, err)
		}
		pattern, err := syscall.UTF16FromString(filter.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid dialog filter pattern %q: %w", filter.Pattern, err)
		}
		prepared.Names = append(prepared.Names, name)
		prepared.Patterns = append(prepared.Patterns, pattern)
		prepared.Specs = append(prepared.Specs, commonDialogFilterSpec{
			Name: &name[0],
			Spec: &pattern[0],
		})
	}

	return prepared, nil
}
