package osutil_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Fepozopo/gokit/osutil"
)

func TestCopyFilePreservesModeAndContent(t *testing.T) {
	td := t.TempDir()

	src := filepath.Join(td, "src.txt")
	dst := filepath.Join(td, "dst.txt")
	content := []byte("hello copy")

	if err := os.WriteFile(src, content, 0o640); err != nil {
		t.Fatalf("write src: %v", err)
	}
	// Ensure the source has the intended mode.
	if err := os.Chmod(src, 0o640); err != nil {
		t.Fatalf("chmod src: %v", err)
	}

	if err := osutil.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q want %q", string(got), string(content))
	}

	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("mode mismatch: got %04o want %04o", fi.Mode().Perm(), 0o640)
	}
}

func TestAtomicReplaceWithExistingDestPreservesMode(t *testing.T) {
	td := t.TempDir()

	dest := filepath.Join(td, "app.bin")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	// Ensure dest mode is set as intended.
	if err := os.Chmod(dest, 0o600); err != nil {
		t.Fatalf("chmod dest: %v", err)
	}

	src := filepath.Join(td, "tmp-new")
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := osutil.AtomicReplace(src, dest); err != nil {
		t.Fatalf("AtomicReplace failed: %v", err)
	}

	// src should be removed
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected src to be removed; stat error = %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("dest content mismatch: got %q want %q", string(got), "new")
	}

	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	// mode should have been preserved from the original dest (0o600)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode mismatch after replace: got %04o want %04o", fi.Mode().Perm(), 0o600)
	}
}

func TestAtomicReplaceWhenDestMissingSetsExecBit(t *testing.T) {
	td := t.TempDir()

	src := filepath.Join(td, "tmp-new2")
	if err := os.WriteFile(src, []byte("fresh"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dest := filepath.Join(td, "newbin")
	if err := osutil.AtomicReplace(src, dest); err != nil {
		t.Fatalf("AtomicReplace failed: %v", err)
	}

	// src should be removed
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected src to be removed; stat error = %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "fresh" {
		t.Fatalf("dest content mismatch: got %q want %q", string(got), "fresh")
	}

	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	// When destination did not exist, AtomicReplace sets user-exec bit (0o755)
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode mismatch for new dest: got %04o want %04o", fi.Mode().Perm(), 0o755)
	}
}

func TestIsCrossDeviceErr(t *testing.T) {
	// Direct syscall.EXDEV should be recognized
	if !osutil.IsCrossDeviceErr(syscall.EXDEV) {
		t.Fatalf("expected EXDEV to be recognized as cross-device error")
	}

	// An *os.LinkError wrapping EXDEV should be recognized
	lerr := &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV}
	if !osutil.IsCrossDeviceErr(lerr) {
		t.Fatalf("expected LinkError(EXDEV) to be recognized as cross-device error")
	}

	// A random error must not be recognized
	if osutil.IsCrossDeviceErr(errors.New("not exdev")) {
		t.Fatalf("unexpectedly recognized unrelated error as cross-device")
	}
}
