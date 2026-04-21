package main

import (
	"fmt"
	"log"

	"github.com/Fepozopo/gokit/osutil"
)

// Simple example program demonstrating the OpenFilePicker and OpenFilesPicker
// helpers in the osutil package.
//
// Usage:
//
//	go run ./_examples/file_eg.go
//
// Notes:
// - These functions will attempt to open the native file-picker on each OS.
// - If the user cancels the dialog, the functions return an empty result with a nil error.
func main() {
	fmt.Println("=== Example: OpenFilePicker (single file) ===")
	sel, err := osutil.OpenFileSelection("Select a file to open")
	if err != nil {
		log.Fatalf("OpenFilePicker error: %v", err)
	}
	if sel == "" {
		fmt.Println("No file selected (canceled).")
	} else {
		fmt.Println("Selected file:", sel)
	}

	fmt.Println()
	fmt.Println("=== Example: OpenFilesPicker (multiple files) ===")
	multi, err := osutil.OpenFilesSelection("Select one or more files")
	if err != nil {
		log.Fatalf("OpenFilesPicker error: %v", err)
	}
	if len(multi) == 0 {
		fmt.Println("No files selected (canceled).")
	} else {
		fmt.Println("Selected files:")
		for i, p := range multi {
			fmt.Printf("  %d) %s\n", i+1, p)
		}
	}

	fmt.Println()
	fmt.Println("=== Example: OpenDirPicker (single directory) ===")
	dir, err := osutil.OpenDirSelection("Select a directory to open")
	if err != nil {
		log.Fatalf("OpenDirPicker error: %v", err)
	}
	if dir == "" {
		fmt.Println("No directory selected (canceled).")
	} else {
		fmt.Println("Selected directory:", dir)
	}

	fmt.Println()
	fmt.Println("=== Example: OpenDirsPicker (multiple directories) ===")
	dirs, err := osutil.OpenDirsSelection("Select one or more directories")
	if err != nil {
		log.Fatalf("OpenDirsPicker error: %v", err)
	}
	if len(dirs) == 0 {
		fmt.Println("No directories selected (canceled).")
	} else {
		fmt.Println("Selected directories:")
		for i, p := range dirs {
			fmt.Printf("  %d) %s\n", i+1, p)
		}
	}

}
