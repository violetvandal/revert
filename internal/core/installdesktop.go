package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InstallDesktop makes the toolkit launchable like a normal installed app. On Linux it
// delegates to the bash dispatcher (which writes a freedesktop .desktop); on Windows it
// creates Start Menu (and Desktop) shortcuts to revert-gui.exe so the user can launch the
// edition from the Start Menu instead of digging into the extracted folder.
func InstallDesktop(c *Conf) error {
	if IsLinux() {
		return DelegateToBash(c.Root, "install-desktop")
	}
	if IsMac() {
		return installDesktopMac(c)
	}
	return installShortcutsWindows(c)
}

func installShortcutsWindows(c *Conf) error {
	gui := filepath.Join(c.Root, "revert-gui.exe")
	if !fileExists(gui) {
		return fmt.Errorf("revert-gui.exe not found next to the toolkit (%s)", c.Root)
	}
	const name = "THUG2 Violet Vandal Edition.lnk"

	targets := []string{}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		targets = append(targets, filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs", name))
	}
	if up := os.Getenv("USERPROFILE"); up != "" {
		targets = append(targets, filepath.Join(up, "Desktop", name))
	}
	if len(targets) == 0 {
		return fmt.Errorf("could not resolve the Start Menu / Desktop location")
	}

	made := 0
	for _, lnk := range targets {
		if err := createShortcut(lnk, gui, c.Root); err != nil {
			note("could not create shortcut " + filepath.Base(filepath.Dir(lnk)) + ": " + err.Error())
			continue
		}
		made++
	}
	if made == 0 {
		return fmt.Errorf("no shortcuts created")
	}
	// An in-place update leaves Explorer showing the icon it cached for the old binary.
	refreshShellIcons()
	ok("Start Menu shortcut created (\"THUG2 Violet Vandal Edition\")")
	return nil
}

// createShortcut writes a Windows .lnk via PowerShell's WScript.Shell COM object (no CGO,
// no extra deps). Target is revert-gui.exe, and IconLocation points at index 0 of that
// same exe, which is the icon resource linked in from tools/pack/icon/revert.ico.
func createShortcut(lnk, target, workDir string) error {
	os.MkdirAll(filepath.Dir(lnk), 0o755)
	ps := "$s=(New-Object -ComObject WScript.Shell).CreateShortcut(" + psQuote(lnk) + ");" +
		"$s.TargetPath=" + psQuote(target) + ";" +
		"$s.WorkingDirectory=" + psQuote(workDir) + ";" +
		"$s.IconLocation=" + psQuote(target+",0") + ";" +
		"$s.Description='Install, build, and play THUG2: Violet Vandal Edition';" +
		"$s.Save()"
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// psQuote single-quotes a string for PowerShell (doubling any embedded single quotes).
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
