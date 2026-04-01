package osutil

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// CopyFile copies a file from src to dst, preserving permissions.
// Used as a fallback when atomic rename is not possible (e.g. cross-device).
// Returns an error on failure.
func CopyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src failed: %w", err)
	}
	defer func() {
		if cerr := sf.Close(); cerr != nil {
			slog.Warn("close src failed", "src", src, "err", cerr)
		}
	}()
	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat src failed: %w", err)
	}
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode())
	if err != nil {
		return fmt.Errorf("open dst failed: %w", err)
	}
	defer func() {
		if cerr := df.Close(); cerr != nil {
			slog.Warn("close dst failed", "dst", dst, "err", cerr)
		}
	}()
	if _, err := io.Copy(df, sf); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}
	if err := df.Sync(); err != nil {
		return fmt.Errorf("sync dst failed: %w", err)
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
