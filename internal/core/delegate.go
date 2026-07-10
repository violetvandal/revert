package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// DelegateToBash runs the proven bash dispatcher `<root>/revert <cmd> <args...>` with
// inherited stdio. This is how every command behaves on Linux/Steam Deck: the Go binary
// is a thin pass-through, so the validated Wine path (share/setup + share/run) is the
// single source of truth there and can never diverge from what this binary would do.
func DelegateToBash(root, cmd string, args ...string) error {
	bash := filepath.Join(root, "revert")
	if !fileExists(bash) {
		return fmt.Errorf("bash dispatcher not found at %s", bash)
	}
	full := append([]string{cmd}, args...)
	c := exec.Command(bash, full...)
	c.Dir = root
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// runInherit runs name with args, inheriting stdio, in dir (cwd if ""), with extra env
// appended to the current environment. Returns the process error (an *exec.ExitError
// carries the child's exit code).
func runInherit(dir string, env []string, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if len(env) > 0 {
		c.Env = append(os.Environ(), env...)
	}
	return c.Run()
}

// ExitCode extracts a process exit code from a run error (0 if nil, the child's code if
// it exited non-zero, 1 otherwise).
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}
