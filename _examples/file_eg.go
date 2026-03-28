package main

import (
	"fmt"
	"log"

	"github.com/Fepozopo/gokit/utils"
)

// Simple example program demonstrating the OpenFilePicker and OpenFilesPicker
// helpers in the utils package.
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
	sel, err := utils.OpenFilePicker("Select a file to open")
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
	multi, err := utils.OpenFilesPicker("Select one or more files")
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
}
