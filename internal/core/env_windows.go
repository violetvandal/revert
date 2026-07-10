//go:build windows

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pad0Configured reports whether the controller is bound — i.e. the registry pad0 value
// exists (written by setup/calibrate). Used as the "setup done" signal for the GUI.
func pad0Configured() bool {
	out, err := exec.Command("reg", "query", settingsKey, "/v", "pad0").CombinedOutput()
	return err == nil && strings.Contains(string(out), "pad0")
}

// directXPresent reports whether the legacy DirectX 9 helper runtime (d3dx9_*.dll) is
// installed. THUG2 is a 2004 D3D8/9 title and those DLLs are not shipped with Win10/11
// by default (this is exactly why the Linux path runs `winetricks d3dx9`). We probe both
// the 64-bit System32 and 32-bit SysWOW64 stores — THUG2 is a 32-bit game, so SysWOW64
// is the one that matters, but check both to be safe.
func directXPresent() bool {
	win := os.Getenv("WINDIR")
	if win == "" {
		win = `C:\Windows`
	}
	for _, sys := range []string{"SysWOW64", "System32"} {
		matches, _ := filepath.Glob(filepath.Join(win, sys, "d3dx9_*.dll"))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// thugProInstalled reports whether THUG Pro (the native Windows online client) is present
// at its standard %LOCALAPPDATA%\THUG Pro location.
func thugProInstalled() bool {
	dir := thugProDir()
	return dir != "" && fileExists(filepath.Join(dir, "THUGProLauncher.exe"))
}

// thugProDir is the native THUG Pro install directory (%LOCALAPPDATA%\THUG Pro).
func thugProDir() string {
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		return filepath.Join(la, "THUG Pro")
	}
	return ""
}
