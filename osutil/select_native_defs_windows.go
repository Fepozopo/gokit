//go:build windows

package osutil

import "syscall"

// This file contains Windows constants, GUIDs, and small ABI/data definitions
// used by the native Common Item Dialog implementation.

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

// CLSIDs and IIDs that identify the COM classes/interfaces used here.
var (
	clsidFileOpenDialog = syscall.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = syscall.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

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
