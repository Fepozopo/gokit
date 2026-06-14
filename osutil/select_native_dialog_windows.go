//go:build windows

package osutil

import (
	"fmt"
	"syscall"
	"unsafe"
)

// This file contains low-level COM interface wrappers for the Windows Common
// Item Dialog APIs used by the native selection backend.

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
