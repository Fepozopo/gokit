package osutil

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// CopyFile copies a file from src to dst, preserving permissions.
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
