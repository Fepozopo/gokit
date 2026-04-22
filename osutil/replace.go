package osutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// AtomicReplace replaces dst with src atomically when possible. It preserves
// file mode when dst exists, does a copy fallback on cross-device errors,
// handles Windows semantics (remove-then-rename), and attempts to fsync the
// containing directory (best-effort).
//
// The caller is responsible for creating src (typically as a temporary file in
// the same directory as dst). On success src will no longer exist. On failure
// this function will attempt best-effort cleanup where appropriate.
func AtomicReplace(src, dst string) error {
	// Preserve mode if destination exists; otherwise ensure executable bit for user.
	if fi, err := os.Stat(dst); err == nil {
		_ = os.Chmod(src, fi.Mode())
	} else {
		_ = os.Chmod(src, 0o755)
	}

	if err := os.Rename(src, dst); err != nil {
		// cross-device fallback
		if IsCrossDeviceErr(err) {
			if cerr := CopyFile(src, dst); cerr != nil {
				return fmt.Errorf("copy fallback failed: %w (rename err: %v)", cerr, err)
			}
			_ = os.Remove(src)
		} else if runtime.GOOS == "windows" {
			// On Windows, try removing dst then renaming
			_ = os.Remove(dst)
			if rerr := os.Rename(src, dst); rerr != nil {
				return fmt.Errorf("rename after remove failed: %w", rerr)
			}
		} else {
			return fmt.Errorf("rename failed: %w", err)
		}
	}

	// fsync containing directory (best-effort)
	dir := filepath.Dir(dst)
	if dirf, err := os.Open(dir); err == nil {
		_ = dirf.Sync()
		_ = dirf.Close()
	}

	return nil
}

// IsCrossDeviceErr reports whether err represents a cross-device rename error (EXDEV).
// It prefers errors.Is for wrapped errors and falls back to inspecting an
// *os.LinkError's underlying errno.
func IsCrossDeviceErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	var lerr *os.LinkError
	if errors.As(err, &lerr) {
		if errno, ok := lerr.Err.(syscall.Errno); ok && errno == syscall.EXDEV {
			return true
		}
	}
	return false
}
