package core

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

// SetupOptions mirror `revert setup`.
type SetupOptions struct {
	Online bool // also set up the THUG Pro online lane
}

// Setup prepares the system to run the edition. On Linux it delegates to the heavy
// share/setup/revert-setup.sh (Wine/DXVK/winetricks/controller/prefixes). On Windows
// almost all of that collapses — THUG2 runs natively — so setup is minimal: ensure the
// DirectX 9 runtime, import the controller bindings, and verify the user-supplied inputs.
func Setup(c *Conf, o SetupOptions) error {
	if !IsWindows() {
		args := []string{}
		if o.Online {
			args = append(args, "--online")
		}
		return DelegateToBash(c.Root, "setup", args...)
	}
	return setupWindows(c, o)
}

func setupWindows(c *Conf, o SetupOptions) error {
	fmt.Println("[revert] Windows setup — native (no Wine needed)")

	// 1. DirectX 9 runtime (d3dx9_*.dll) — the one real native dependency for a 2004 game.
	if directXPresent() {
		ok("DirectX 9 runtime already present")
	} else {
		if err := ensureDirectX(c); err != nil {
			note("DirectX 9 not installed automatically: " + err.Error())
			note("Install Microsoft's DirectX End-User Runtime, then re-run: revert doctor")
		}
	}

	// 2. Controller bindings — import the game's DirectInput/keyboard map natively, then
	// calibrate pad0 to THIS machine's real DirectInput instance GUID (the shipped .reg
	// carries only a placeholder; THUG2 opens only the pad whose guidInstance == pad0).
	if err := importControllerReg(c); err != nil {
		note("controller bindings not imported: " + err.Error())
	} else {
		ok("controller bindings imported")
	}
	if err := Calibrate(c); err != nil {
		note("controller not calibrated (plug in the pad, then run: revert calibrate-controller): " + err.Error())
	}

	// 3. Verify the user-supplied build inputs are in place.
	if fileExists(c.Path("NOCD_EXE")) {
		ok("no-CD THUG2.exe present")
	} else {
		note("no-CD THUG2.exe not present (user-supplied): " + c.Path("NOCD_EXE"))
	}
	if fileExists(c.Path("WSFIX_ZIP")) {
		ok("WidescreenFix zip present")
	} else {
		note("WidescreenFix zip not present (user-supplied): " + c.Path("WSFIX_ZIP"))
	}

	if o.Online {
		if thugProInstalled() {
			ok("THUG Pro present (online lane)")
		} else {
			note("THUG Pro not installed — run its setup (THUGProSetup.exe) for the online lane")
		}
	}

	// 4. Make it launchable like a normal app (Start Menu + Desktop shortcuts).
	if err := InstallDesktop(c); err != nil {
		note("app shortcut not created: " + err.Error())
	}

	fmt.Println("[revert] setup done. Next: revert acquire-game-data --from <your THUG2 folder> ; revert build ; revert run qol")
	return nil
}

// ensureDirectX runs a bundled DirectX End-User Runtime installer if one is present at
// tools/dx/ (kept out of git like all binaries; the redist is freely redistributable but
// user-supplied). Returns an error if none is bundled.
func ensureDirectX(c *Conf) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("not windows")
	}
	for _, name := range []string{"DXSETUP.exe", "dxwebsetup.exe"} {
		p := filepath.Join(c.Root, "tools", "dx", name)
		if fileExists(p) {
			fmt.Printf("[revert] running DirectX runtime installer: %s\n", name)
			args := []string{}
			if name == "DXSETUP.exe" {
				args = []string{"/silent"}
			}
			return runInherit(filepath.Dir(p), nil, p, args...)
		}
	}
	return fmt.Errorf("no bundled DirectX installer under tools/dx/")
}

// importControllerReg imports the THUG2 controller/keyboard bindings via the native
// `reg import`. Prefers a Windows-specific .reg, falling back to the shared one (its key
// path HKCU\Software\Activision\... is already the real Windows path).
func importControllerReg(c *Conf) error {
	controls := filepath.Join(c.Root, "tools", "controls")
	candidates := []string{
		filepath.Join(controls, "thug2-settings-windows.reg"),
		filepath.Join(controls, "thug2-settings.reg"),
	}
	var reg string
	for _, p := range candidates {
		if fileExists(p) {
			reg = p
			break
		}
	}
	if reg == "" {
		return fmt.Errorf("no controller .reg found under %s", controls)
	}
	out, err := exec.Command("reg", "import", reg).CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg import: %v (%s)", err, string(out))
	}
	return nil
}
