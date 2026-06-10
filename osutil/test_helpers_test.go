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
