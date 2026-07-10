package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// FindRoot resolves the toolkit root (the dir holding revert.conf): $REVERT_ROOT wins,
// else the directory of the running executable and its parents (the bundle ships
// revert.exe next to revert.conf), else the working directory and its parent (dev:
// `go run ./cmd/revert` from the repo). Returns "" if not found.
func FindRoot() string {
	if r := os.Getenv("REVERT_ROOT"); r != "" {
		return absOr(r)
	}
	var cands []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands, d, filepath.Dir(d), filepath.Dir(filepath.Dir(d)))
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands, wd, filepath.Dir(wd))
	}
	for _, c := range cands {
		if fileExists(filepath.Join(c, "revert.conf")) {
			return absOr(c)
		}
	}
	return ""
}

// LoadRootConf finds the root and loads revert.conf, erroring clearly if either fails.
func LoadRootConf() (*Conf, error) {
	root := FindRoot()
	if root == "" {
		return nil, fmt.Errorf("could not locate the toolkit root (no revert.conf found near the executable or working directory); set REVERT_ROOT")
	}
	c, err := LoadConf(filepath.Join(root, "revert.conf"), root)
	if err != nil {
		return nil, fmt.Errorf("reading revert.conf: %w", err)
	}
	// Machine-specific overrides, sourced last exactly as the bash dispatcher does. Absent
	// on most installs; gitignored, and never replaced by `revert update`.
	if local := filepath.Join(root, "revert.conf.local"); fileExists(local) {
		if err := c.Overlay(local); err != nil {
			return nil, fmt.Errorf("reading revert.conf.local: %w", err)
		}
	}
	return c, nil
}

func absOr(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// IsWindows reports whether we're the native-Windows front door (vs. the Linux path
// that delegates to the bash dispatcher).
func IsWindows() bool { return runtime.GOOS == "windows" }
