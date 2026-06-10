package osutil

import (
	"bytes"
	"os/exec"
)

var commandExec = exec.Command
var lookPathExec = exec.LookPath

// hasCommand reports whether name can be found in PATH.
func hasCommand(name string) bool {
	_, err := lookPathExec(name)
	return err == nil
}

// runCommandOutput executes name with args and returns stdout.
func runCommandOutput(name string, args ...string) ([]byte, error) {
	return commandExec(name, args...).Output()
}

// runCommandCombined executes name with args and returns combined stdout and stderr.
func runCommandCombined(name string, args ...string) (string, error) {
	cmd := commandExec(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}
