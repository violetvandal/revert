package core

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// DelegateToBash runs the proven bash dispatcher `<root>/revert <cmd> <args...>` with
// inherited stdio. This is how every command behaves on Linux/Steam Deck: the Go binary
// is a thin pass-through, so the validated Wine path (share/setup + share/run) is the
// single source of truth there and can never diverge from what this binary would do.
func DelegateToBash(root, cmd string, args ...string) error {
	// Backstop against an exec loop. On Darwin the bash `revert` immediately execs THIS
	// binary, so delegating back to it would ping-pong forever. Every command is supposed
	// to branch on IsLinux() before reaching here; if one ever forgets, fail loudly rather
	// than fork-bomb the user's Mac.
	if IsMac() {
		return fmt.Errorf("internal: `%s` tried to delegate to the bash dispatcher on macOS, "+
			"which delegates straight back — this command needs a native macOS branch", cmd)
	}
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

// runTee is runInherit that also copies the child's stdout+stderr into logw, so the last
// launch survives on disk for `revert report` to quote. The user still sees everything
// live; logw is a second destination, not a redirection.
//
// A nil logw makes this exactly runInherit, which is the point: every caller can ask for
// a log unconditionally and a failure to open the log file degrades to "no log" rather
// than to "the game will not start".
func runTee(logw io.Writer, dir string, env []string, name string, args ...string) error {
	if logw == nil {
		return runInherit(dir, env, name, args...)
	}
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdin = os.Stdin
	// Both pipes must funnel into logw through ONE synchronized writer.
	//
	// os/exec copies stdout and stderr on separate goroutines whenever they are not
	// *os.File, so handing the same writer to both MultiWriters means two goroutines
	// writing to it concurrently: a data race, and interleaved half-lines in the log even
	// when the writer happens to be race-free. A garbled log is worse than none, because
	// it is read as evidence.
	safe := &syncWriter{w: logw}
	c.Stdout = io.MultiWriter(os.Stdout, safe)
	c.Stderr = io.MultiWriter(os.Stderr, safe)
	if len(env) > 0 {
		c.Env = append(os.Environ(), env...)
	}
	return c.Run()
}

// syncWriter serializes concurrent writes to an underlying writer.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
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
