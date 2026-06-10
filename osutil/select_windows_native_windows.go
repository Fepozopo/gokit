//go:build windows

package osutil

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// Windows and COM constants used by the Common Item Dialog interop layer.
const (
	coinitApartmentThreaded = 0x2
	sFalse                  = 0x00000001
	rpcEChangedMode         = 0x80010106
	hresultCanceled         = 0x800704C7
	clsctxInprocServer      = 0x1

	fosNoChangeDir      = 0x00000008
	fosPickFolders      = 0x00000020
	fosForceFilesystem  = 0x00000040
	fosAllowMultiSelect = 0x00000200
	fosPathMustExist    = 0x00000800
	fosFileMustExist    = 0x00001000

	sigdnFileSysPath = 0x80058000
)

// errNativeDialogCancelled marks a native Windows dialog dismissal without a
// selection.
//
// The public `osutil` selection helpers intentionally normalize cancellation to
// an empty result with a nil error, but the Windows COM layer still needs a
// precise internal signal so cancellation can be distinguished from real COM
// failures while the dialog flow unwinds.
var errNativeDialogCancelled = errors.New("cancelled")

// Lazily-resolved Windows DLLs and procedures used by the dialog helpers.
var (
	ole32DLL = syscall.NewLazyDLL("ole32.dll")

	procCoCreateInstance = ole32DLL.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32DLL.NewProc("CoTaskMemFree")
	procCoInitializeEx   = ole32DLL.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32DLL.NewProc("CoUninitialize")
)

// CLSIDs and IIDs that identify the COM classes/interfaces used here.
var (
	clsidFileOpenDialog = syscall.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = syscall.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

// COM interface and data structure definitions.

// fileOpenDialog represents the COM `IFileOpenDialog` interface used by the
// Windows Common Item Dialog API.
//
// Only the methods needed by this package are wrapped below. The full vtable is
// still described so the method offsets match the Windows ABI exactly.
type fileOpenDialog struct {
	vtbl *fileOpenDialogVTable
}

// fileOpenDialogVTable mirrors the `IFileOpenDialog` vtable layout.
//
// The placeholder entries are intentionally kept in order because COM dispatch
// depends on exact method positions rather than method names.
type fileOpenDialogVTable struct {
	QueryInterface      uintptr
	AddRef              uintptr
	Release             uintptr
	Show                uintptr
	SetFileTypes        uintptr
	SetFileTypeIndex    uintptr
	GetFileTypeIndex    uintptr
	Advise              uintptr
	Unadvise            uintptr
	SetOptions          uintptr
	GetOptions          uintptr
	SetDefaultFolder    uintptr
	SetFolder           uintptr
	GetFolder           uintptr
	GetCurrentSelection uintptr
	SetFileName         uintptr
	GetFileName         uintptr
	SetTitle            uintptr
	SetOkButtonLabel    uintptr
	SetFileNameLabel    uintptr
	GetResult           uintptr
	AddPlace            uintptr
	SetDefaultExtension uintptr
	Close               uintptr
	SetClientGuid       uintptr
	ClearClientData     uintptr
	SetFilter           uintptr
	GetResults          uintptr
	GetSelectedItems    uintptr
}

// shellItem represents the COM `IShellItem` interface returned by the Common
// Item Dialog after the user chooses a file or folder.
type shellItem struct {
	vtbl *shellItemVTable
}

// shellItemVTable mirrors the `IShellItem` vtable layout.
//
// As with the dialog vtable, the entries must stay in the Windows-defined order
// so calls land on the correct COM methods.
type shellItemVTable struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	BindToHandler  uintptr
	GetParent      uintptr
	GetDisplayName uintptr
	GetAttributes  uintptr
	Compare        uintptr
}

// shellItemArray represents the COM `IShellItemArray` interface used when the
// dialog allows selecting multiple files or folders.
//
// Windows returns this interface from `IFileOpenDialog::GetResults`, and the
// code below walks the array to resolve each selected entry into a filesystem
// path.
type shellItemArray struct {
	vtbl *shellItemArrayVTable
}

// shellItemArrayVTable mirrors the `IShellItemArray` vtable layout.
//
// Only a few methods are used directly, but the order must still match the COM
// ABI so calls to `GetCount` and `GetItemAt` land on the correct entries.
type shellItemArrayVTable struct {
	QueryInterface             uintptr
	AddRef                     uintptr
	Release                    uintptr
	BindToHandler              uintptr
	GetPropertyStore           uintptr
	GetPropertyDescriptionList uintptr
	GetAttributes              uintptr
	GetCount                   uintptr
	GetItemAt                  uintptr
	EnumItems                  uintptr
}

// commonDialogFilterSpec mirrors Windows `COMDLG_FILTERSPEC`.
//
// The Common Item Dialog consumes a pointer to an array of these structs when a
// caller configures named file type filters.
type commonDialogFilterSpec struct {
	Name *uint16
	Spec *uint16
}

// fileDialogFilter describes a single file type option shown by the Windows
// Common Item Dialog.
type fileDialogFilter struct {
	DisplayName string
	Pattern     string
}

// preparedDialogFilters keeps the UTF-16 backing storage for filter strings
// alive while the dialog is being configured.
//
// The COM API receives raw pointers into this storage, so the slices must remain
// reachable until `SetFileTypes` returns.
type preparedDialogFilters struct {
	Specs    []commonDialogFilterSpec
	Names    [][]uint16
	Patterns [][]uint16
}

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

// initializeCOM prepares the current thread for shell APIs that require COM.
//
// It returns a cleanup function only when this call successfully performed a new
// COM initialization. When COM is already initialized in a different apartment
// model, Windows reports `RPC_E_CHANGED_MODE`; that state is still usable for
// the dialogs here, so the function treats it as success without registering a
// cleanup callback.
func initializeCOM() (func(), error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	switch uint32(hr) {
	case 0:
		return func() {
			procCoUninitialize.Call()
		}, nil
	case sFalse:
		return func() {
			procCoUninitialize.Call()
		}, nil
	case rpcEChangedMode:
		return nil, nil
	default:
		return nil, fmt.Errorf("CoInitializeEx failed with HRESULT 0x%X", uint32(hr))
	}
}

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

// createFileOpenDialog constructs an `IFileOpenDialog` COM instance.
//
// Keeping COM object creation in a dedicated helper centralizes the CLSID/IID
// wiring and makes the higher-level picker flow easier to read.
func createFileOpenDialog() (*fileOpenDialog, error) {
	var dialog *fileOpenDialog
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&dialog)),
	)
	if failedHRESULT(hr) {
		return nil, hresultError{Method: "CoCreateInstance(IFileOpenDialog)", Code: uint32(hr)}
	}
	if dialog == nil {
		return nil, fmt.Errorf("CoCreateInstance(IFileOpenDialog) returned a nil dialog")
	}
	return dialog, nil
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

		name := syscall.StringToUTF16(filter.DisplayName)
		pattern := syscall.StringToUTF16(filter.Pattern)
		prepared.Names = append(prepared.Names, name)
		prepared.Patterns = append(prepared.Patterns, pattern)
		prepared.Specs = append(prepared.Specs, commonDialogFilterSpec{
			Name: &name[0],
			Spec: &pattern[0],
		})
	}

	return prepared, nil
}

// `IFileOpenDialog` COM method wrappers.

// Options returns the dialog's current `FILEOPENDIALOGOPTIONS` flags.
func (dialog *fileOpenDialog) Options() (uint32, error) {
	var options uint32
	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.GetOptions,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(unsafe.Pointer(&options)),
	)
	if failedHRESULT(hr) {
		return 0, hresultError{Method: "IFileOpenDialog::GetOptions", Code: uint32(hr)}
	}
	return options, nil
}

// SetOptions updates the dialog's `FILEOPENDIALOGOPTIONS` flags.
func (dialog *fileOpenDialog) SetOptions(options uint32) error {
	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.SetOptions,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(options),
	)
	if failedHRESULT(hr) {
		return hresultError{Method: "IFileOpenDialog::SetOptions", Code: uint32(hr)}
	}
	return nil
}

// SetFileTypes configures the named file filters shown in the dialog's type
// dropdown.
//
// Windows expects at least one `COMDLG_FILTERSPEC`. The caller is responsible
// for keeping the backing UTF-16 strings alive until this method returns.
func (dialog *fileOpenDialog) SetFileTypes(filters []commonDialogFilterSpec) error {
	if len(filters) == 0 {
		return fmt.Errorf("IFileOpenDialog::SetFileTypes requires at least one filter")
	}

	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.SetFileTypes,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(len(filters)),
		uintptr(unsafe.Pointer(&filters[0])),
	)
	if failedHRESULT(hr) {
		return hresultError{Method: "IFileOpenDialog::SetFileTypes", Code: uint32(hr)}
	}
	return nil
}

// SetFileTypeIndex selects which configured filter should be active when the
// dialog first opens.
//
// The Common Item Dialog uses a one-based index, so `1` selects the first
// filter.
func (dialog *fileOpenDialog) SetFileTypeIndex(index uint32) error {
	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.SetFileTypeIndex,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(index),
	)
	if failedHRESULT(hr) {
		return hresultError{Method: "IFileOpenDialog::SetFileTypeIndex", Code: uint32(hr)}
	}
	return nil
}

// SetDefaultExtension configures the extension that Windows should suggest when
// a user enters a filename without one.
//
// This helper is currently unused by `osutil`, but it remains documented and
// available because it is part of the already-modeled `IFileOpenDialog` surface
// and may be useful for future filter-aware dialog behavior.
func (dialog *fileOpenDialog) SetDefaultExtension(extension *uint16) error {
	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.SetDefaultExtension,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(unsafe.Pointer(extension)),
	)
	if failedHRESULT(hr) {
		return hresultError{Method: "IFileOpenDialog::SetDefaultExtension", Code: uint32(hr)}
	}
	return nil
}

// SetTitle sets the caption shown at the top of the dialog window.
func (dialog *fileOpenDialog) SetTitle(title *uint16) error {
	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.SetTitle,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(unsafe.Pointer(title)),
	)
	if failedHRESULT(hr) {
		return hresultError{Method: "IFileOpenDialog::SetTitle", Code: uint32(hr)}
	}
	return nil
}

// Show displays the dialog modally for the provided owner window handle.
func (dialog *fileOpenDialog) Show(owner uintptr) error {
	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.Show,
		uintptr(unsafe.Pointer(dialog)),
		owner,
	)
	if failedHRESULT(hr) {
		return hresultError{Method: "IFileOpenDialog::Show", Code: uint32(hr)}
	}
	return nil
}

// Result returns the shell item selected when the dialog closes successfully.
func (dialog *fileOpenDialog) Result() (*shellItem, error) {
	var item *shellItem
	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.GetResult,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(unsafe.Pointer(&item)),
	)
	if failedHRESULT(hr) {
		return nil, hresultError{Method: "IFileOpenDialog::GetResult", Code: uint32(hr)}
	}
	if item == nil {
		return nil, fmt.Errorf("IFileOpenDialog::GetResult returned a nil shell item")
	}
	return item, nil
}

// Results returns the shell item array selected when the dialog closes in a
// multi-select mode.
//
// The Common Item Dialog exposes multi-selection through `IShellItemArray`
// rather than through repeated calls to `GetResult`.
func (dialog *fileOpenDialog) Results() (*shellItemArray, error) {
	var items *shellItemArray
	hr, _, _ := syscall.SyscallN(
		dialog.vtbl.GetResults,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(unsafe.Pointer(&items)),
	)
	if failedHRESULT(hr) {
		return nil, hresultError{Method: "IFileOpenDialog::GetResults", Code: uint32(hr)}
	}
	if items == nil {
		return nil, fmt.Errorf("IFileOpenDialog::GetResults returned a nil shell item array")
	}
	return items, nil
}

// Release decrements the COM reference count for the dialog.
//
// The helper tolerates nil receivers so callers can safely defer it after a
// successful creation check.
func (dialog *fileOpenDialog) Release() {
	if dialog == nil {
		return
	}
	syscall.SyscallN(
		dialog.vtbl.Release,
		uintptr(unsafe.Pointer(dialog)),
	)
}

// `IShellItem` COM method wrappers.

// DisplayName returns a filesystem path for the shell item when one is
// available.
//
// The Common Item Dialog may surface non-filesystem shell items in some modes,
// so the caller requests `SIGDN_FILESYSPATH` explicitly to guarantee a usable
// path for the application.
func (item *shellItem) DisplayName(sigdn uint32) (string, error) {
	var rawPath *uint16
	hr, _, _ := syscall.SyscallN(
		item.vtbl.GetDisplayName,
		uintptr(unsafe.Pointer(item)),
		uintptr(sigdn),
		uintptr(unsafe.Pointer(&rawPath)),
	)
	if failedHRESULT(hr) {
		return "", hresultError{Method: "IShellItem::GetDisplayName", Code: uint32(hr)}
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(rawPath)))

	selected := utf16PtrToString(rawPath)
	if selected == "" {
		return "", errNativeDialogCancelled
	}
	return selected, nil
}

// Release decrements the COM reference count for the shell item.
func (item *shellItem) Release() {
	if item == nil {
		return
	}
	syscall.SyscallN(
		item.vtbl.Release,
		uintptr(unsafe.Pointer(item)),
	)
}

// `IShellItemArray` COM method wrappers.

// Count reports how many shell items were returned by a multi-select dialog.
func (items *shellItemArray) Count() (uint32, error) {
	var count uint32
	hr, _, _ := syscall.SyscallN(
		items.vtbl.GetCount,
		uintptr(unsafe.Pointer(items)),
		uintptr(unsafe.Pointer(&count)),
	)
	if failedHRESULT(hr) {
		return 0, hresultError{Method: "IShellItemArray::GetCount", Code: uint32(hr)}
	}
	return count, nil
}

// ItemAt returns the shell item at the requested zero-based index.
func (items *shellItemArray) ItemAt(index uint32) (*shellItem, error) {
	var item *shellItem
	hr, _, _ := syscall.SyscallN(
		items.vtbl.GetItemAt,
		uintptr(unsafe.Pointer(items)),
		uintptr(index),
		uintptr(unsafe.Pointer(&item)),
	)
	if failedHRESULT(hr) {
		return nil, hresultError{Method: "IShellItemArray::GetItemAt", Code: uint32(hr)}
	}
	if item == nil {
		return nil, fmt.Errorf("IShellItemArray::GetItemAt returned a nil shell item")
	}
	return item, nil
}

// DisplayNames resolves every shell item in the array into a filesystem path.
//
// This helper keeps the higher-level selection flow independent from the COM
// collection interface by eagerly converting the entire array into a Go slice.
func (items *shellItemArray) DisplayNames(sigdn uint32) ([]string, error) {
	count, err := items.Count()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}

	selected := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		item, err := items.ItemAt(i)
		if err != nil {
			return nil, err
		}
		name, nameErr := item.DisplayName(sigdn)
		item.Release()
		if nameErr != nil {
			return nil, nameErr
		}
		if name != "" {
			selected = append(selected, name)
		}
	}
	if len(selected) == 0 {
		return nil, errNativeDialogCancelled
	}
	return selected, nil
}

// Release decrements the COM reference count for the shell item array.
func (items *shellItemArray) Release() {
	if items == nil {
		return
	}
	syscall.SyscallN(
		items.vtbl.Release,
		uintptr(unsafe.Pointer(items)),
	)
}

// Error and string conversion helpers.

// hresultError wraps a failing Windows HRESULT with the method that returned
// it.
//
// Using a typed error keeps cancellation handling precise without sacrificing
// readable error messages for other COM failures.
type hresultError struct {
	Method string
	Code   uint32
}

// Error formats the failing COM call in a way that is readable in logs and test
// output.
func (err hresultError) Error() string {
	return fmt.Sprintf("%s failed with HRESULT 0x%X", err.Method, err.Code)
}

// failedHRESULT reports whether an HRESULT indicates failure.
//
// In COM, any negative signed HRESULT value represents an error condition.
func failedHRESULT(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}

// utf16PtrToString converts a Windows-owned UTF-16 string pointer into a Go
// string.
//
// COM returns many textual results as NUL-terminated UTF-16 pointers allocated
// by the callee. This helper reads until the terminating NUL and leaves memory
// ownership to the caller, which can then release the original allocation.
func utf16PtrToString(ptr *uint16) string {
	if ptr == nil {
		return ""
	}

	length := 0
	for current := ptr; *current != 0; length++ {
		// Advance one UTF-16 code unit at a time until the terminating NUL.
		current = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(current)) + unsafe.Sizeof(*current)))
	}

	buffer := unsafe.Slice(ptr, length)
	return syscall.UTF16ToString(buffer)
}
