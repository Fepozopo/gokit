package osutil

import (
	"os/exec"
	"reflect"
	"testing"
)

// TestClipboardCommandSpecLinuxPrefersXclipThenWlCopy verifies Linux backend
// selection prefers xclip before wl-copy when both are available.
func TestClipboardCommandSpecLinuxPrefersXclipThenWlCopy(t *testing.T) {
	setOSUtilTestGlobals(t, "linux", func(name string) (string, error) {
		switch name {
		case "xclip":
			return "/usr/bin/xclip", nil
		case "wl-copy":
			return "/usr/bin/wl-copy", nil
		default:
			return "", exec.ErrNotFound
		}
	}, nil)

	name, args, err := clipboardCommandSpec("linux")
	if err != nil {
		t.Fatalf("clipboardCommandSpec returned error: %v", err)
	}
	if name != "xclip" {
		t.Fatalf("name = %q, want %q", name, "xclip")
	}
	if !reflect.DeepEqual(args, []string{"-selection", "clipboard"}) {
		t.Fatalf("args = %#v, want %#v", args, []string{"-selection", "clipboard"})
	}

	setOSUtilTestGlobals(t, "linux", func(name string) (string, error) {
		if name == "wl-copy" {
			return "/usr/bin/wl-copy", nil
		}
		return "", exec.ErrNotFound
	}, nil)
	name, args, err = clipboardCommandSpec("linux")
	if err != nil {
		t.Fatalf("clipboardCommandSpec returned error: %v", err)
	}
	if name != "wl-copy" {
		t.Fatalf("name = %q, want %q", name, "wl-copy")
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want no args", args)
	}
}

// TestClipboardCommandSpecReturnsErrorWithoutLinuxBackend verifies Linux
// selection fails when neither clipboard helper is installed.
func TestClipboardCommandSpecReturnsErrorWithoutLinuxBackend(t *testing.T) {
	setOSUtilTestGlobals(t, "linux", func(string) (string, error) {
		return "", exec.ErrNotFound
	}, nil)

	_, _, err := clipboardCommandSpec("linux")
	if err == nil {
		t.Fatal("expected clipboardCommandSpec to fail")
	}
}
