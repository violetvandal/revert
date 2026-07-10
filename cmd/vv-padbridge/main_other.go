//go:build !windows

// vv-padbridge is a Windows-only helper (XInput -> keystroke). On Linux the equivalent
// is the evdev trigger bridge (tools/trigger-bridge/thug2-trigger-bridge.py). This stub
// exists so `go build ./...` succeeds cross-platform.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "vv-padbridge runs on Windows only (Linux uses the evdev trigger bridge)")
	os.Exit(1)
}
