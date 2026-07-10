//go:build !windows

package main

import (
	"io"
	"os/exec"
	"runtime"
)

// openBrowser opens url in the default browser on macOS (open) / Linux (xdg-open).
func openBrowser(url string) {
	var c *exec.Cmd
	if runtime.GOOS == "darwin" {
		c = exec.Command("open", url)
	} else {
		c = exec.Command("xdg-open", url)
	}
	c.Stdout, c.Stderr = io.Discard, io.Discard
	_ = c.Start()
}
