package core

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// Tag passes through to the thugkit build core's `tag` subcommand (custom Create-A-Graphic
// tags). Same on every platform — thugkit is fully cross-platform.
func Tag(c *Conf, args []string) error {
	tk := c.Thugkit()
	if !fileExists(tk) {
		return fmt.Errorf("thugkit binary missing (%s) — build first", tk)
	}
	return runInherit(c.Root, nil, tk, append([]string{"tag"}, args...)...)
}

// LaunchGUI starts the local web-UI installer. On Linux it delegates to the bash
// dispatcher (which builds gui/revert-gui if needed, then launches it). On Windows the
// bundle ships revert-gui.exe next to revert.exe — launch it directly.
func LaunchGUI(c *Conf, args []string) error {
	if IsMac() {
		return runMacGUI(c, args)
	}
	if IsLinux() {
		return DelegateToBash(c.Root, "gui", args...)
	}
	for _, p := range []string{
		filepath.Join(c.Root, "revert-gui.exe"),
		filepath.Join(c.Root, "gui", "revert-gui.exe"),
	} {
		if fileExists(p) {
			return runInherit(c.Root, nil, p, args...)
		}
	}
	return fmt.Errorf("revert-gui.exe not found next to revert.exe — reinstall the Windows bundle")
}

// DelegateOrUnsupported hands a command to the bash dispatcher on Linux, or reports it as
// unsupported on the native-Windows lane (acquire-hq and build-installer are Linux/Deck
// concerns; their Windows equivalents live elsewhere or aren't needed). `update` used to
// live here too — it now has a native implementation in update.go.
func DelegateOrUnsupported(c *Conf, cmd string, args []string) error {
	if IsLinux() {
		return DelegateToBash(c.Root, cmd, args...)
	}
	return fmt.Errorf("`%s` is not available on the native lane (%s)", cmd, runtime.GOOS)
}
