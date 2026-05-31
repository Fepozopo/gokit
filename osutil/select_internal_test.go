package osutil

import (
	"os/exec"
	"testing"
)

// setOSUtilTestGlobals swaps package-level test seams and restores them when the test ends.
func setOSUtilTestGlobals(t *testing.T, goos string, lookPath func(string) (string, error), cmd func(string, ...string) *exec.Cmd) {
	t.Helper()
	oldGOOS := currentGOOS
	oldLookPath := lookPathExec
	oldCommandExec := commandExec
	if goos != "" {
		currentGOOS = goos
	}
	if lookPath != nil {
		lookPathExec = lookPath
	}
	if cmd != nil {
		commandExec = cmd
	}
	t.Cleanup(func() {
		currentGOOS = oldGOOS
		lookPathExec = oldLookPath
		commandExec = oldCommandExec
	})
}

// TestParseSelectionListHandlesNewlinesAndPipes verifies selection parsing accepts
// both newline- and pipe-delimited helper output.
func TestParseSelectionListHandlesNewlinesAndPipes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "newline separated", raw: " /tmp/a\n/tmp/b \n", want: []string{"/tmp/a", "/tmp/b"}},
		{name: "pipe separated", raw: " /tmp/a| /tmp/b |", want: []string{"/tmp/a", "/tmp/b"}},
		{name: "empty", raw: "   ", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSelectionList(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("len(got) = %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestLinuxSelectionBackendPrefersZenityThenKDialog verifies backend discovery
// prefers zenity and otherwise falls back to kdialog.
func TestLinuxSelectionBackendPrefersZenityThenKDialog(t *testing.T) {
	setOSUtilTestGlobals(t, "linux", func(name string) (string, error) {
		switch name {
		case "zenity":
			return "/usr/bin/zenity", nil
		case "kdialog":
			return "/usr/bin/kdialog", nil
		default:
			return "", exec.ErrNotFound
		}
	}, nil)

	if got := linuxSelectionBackend(); got != "zenity" {
		t.Fatalf("backend = %q, want %q", got, "zenity")
	}

	setOSUtilTestGlobals(t, "linux", func(name string) (string, error) {
		if name == "kdialog" {
			return "/usr/bin/kdialog", nil
		}
		return "", exec.ErrNotFound
	}, nil)
	if got := linuxSelectionBackend(); got != "kdialog" {
		t.Fatalf("backend = %q, want %q", got, "kdialog")
	}
}

// TestSelectFileLinuxReturnsErrNoGUISelectionWhenNoBackendExists verifies Linux
// selection fails with ErrNoGUISelection when no GUI helper is installed.
func TestSelectFileLinuxReturnsErrNoGUISelectionWhenNoBackendExists(t *testing.T) {
	setOSUtilTestGlobals(t, "linux", func(string) (string, error) {
		return "", exec.ErrNotFound
	}, nil)

	_, err := selectFileLinux("pick a file")
	if err != ErrNoGUISelection {
		t.Fatalf("err = %v, want %v", err, ErrNoGUISelection)
	}
}

// TestSelectFilesLinuxParsesZenityPipeSeparatedOutput verifies the Linux parser
// tolerates zenity-style output represented with pipe separators.
func TestSelectFilesLinuxParsesZenityPipeSeparatedOutput(t *testing.T) {
	setOSUtilTestGlobals(t, "linux", func(name string) (string, error) {
		if name == "zenity" {
			return "/usr/bin/zenity", nil
		}
		return "", exec.ErrNotFound
	}, func(string, ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "printf '/tmp/a|/tmp/b'")
	})

	got, err := selectFilesLinux("pick files")
	if err != nil {
		t.Fatalf("selectFilesLinux returned error: %v", err)
	}
	want := []string{"/tmp/a", "/tmp/b"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSelectFileLinuxTreatsEmptyErrorOutputAsCancel verifies an empty failing
// helper result is normalized to a user cancellation.
func TestSelectFileLinuxTreatsEmptyErrorOutputAsCancel(t *testing.T) {
	setOSUtilTestGlobals(t, "linux", func(name string) (string, error) {
		if name == "zenity" {
			return "/usr/bin/zenity", nil
		}
		return "", exec.ErrNotFound
	}, func(string, ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	})

	got, err := selectFileLinux("pick file")
	if err != nil {
		t.Fatalf("selectFileLinux returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("got = %q, want empty result on cancel", got)
	}
}
