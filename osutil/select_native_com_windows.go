//go:build windows

package osutil

import (
	"fmt"
	"syscall"
	"unsafe"
)

// This file contains shared COM plumbing for the native Windows dialog backend:
// ole32 procedure bindings, COM initialization, HRESULT handling, and UTF-16
// pointer conversion.

// Lazily-resolved Windows DLLs and procedures used by the dialog helpers.
var (
	ole32DLL = syscall.NewLazyDLL("ole32.dll")

	procCoCreateInstance = ole32DLL.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32DLL.NewProc("CoTaskMemFree")
	procCoInitializeEx   = ole32DLL.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32DLL.NewProc("CoUninitialize")
)

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
