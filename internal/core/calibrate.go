package core

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// settingsKey is THUG2's registry home (same on native Windows and under Wine).
const settingsKey = `HKCU\Software\Activision\Tony Hawk's Underground 2\Settings`

var guidRe = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

// Calibrate detects the connected controller's DirectInput instance GUID and writes it to
// the registry `pad0`, so THUG2 (which only opens the pad whose guidInstance == pad0) binds
// the right device. On Linux it delegates to the bash calibrate step; on Windows it runs the
// native DirectInput probe (dinput_probe_guid.exe) and writes pad0 with `reg add`.
//
// This is the Windows analogue of the Steam Deck's per-prefix GUID problem: the shipped
// thug2-settings.reg carries a placeholder pad0 that is unlikely to match a given machine's
// real DirectInput instance GUID, so it must be detected live.
func Calibrate(c *Conf) error {
	if IsLinux() {
		return DelegateToBash(c.Root, "calibrate-controller")
	}
	// macOS runs the same probe, but under wine and against the lane's prefix.
	if IsMac() {
		return calibrateMac(c, macResolve(c))
	}
	guid, err := detectPadGUID(c)
	if err != nil {
		return err
	}
	if err := setPad0(guid); err != nil {
		return fmt.Errorf("writing pad0: %w", err)
	}
	fmt.Printf("[revert] controller calibrated: pad0 -> %s\n", guid)
	return nil
}

// detectPadGUID runs the DirectInput probe and returns the connected gamepad's guidInstance.
func detectPadGUID(c *Conf) (string, error) {
	probe := probePath(c)
	if probe == "" {
		return "", fmt.Errorf("DirectInput probe not found (expected tools\\xinput-probe\\dinput_probe_guid.exe)")
	}
	out, err := exec.Command(probe).CombinedOutput()
	if err != nil {
		// The probe returns non-zero if DirectInput init fails; still try to parse.
		if len(out) == 0 {
			return "", fmt.Errorf("running the DirectInput probe: %w", err)
		}
	}
	guid := parseGamepadGUID(string(out))
	if guid == "" {
		return "", fmt.Errorf("no game-controller GUID found — is the controller plugged in and on?")
	}
	return guid, nil
}

// probePath resolves the DirectInput probe binary.
func probePath(c *Conf) string {
	if p := c.Path("PAD_PROBE"); fileExists(p) {
		return p
	}
	p := filepath.Join(c.Root, "tools", "xinput-probe", "dinput_probe_guid.exe")
	if fileExists(p) {
		return p
	}
	return ""
}

// parseGamepadGUID extracts the DirectInput guidInstance of the first game controller from
// the probe's output. It prefers a device whose type line says GAMEPAD or JOYSTICK (skipping
// any mouse/keyboard the "ALL" enum pass prints), and falls back to the first guidInstance
// seen. Mirrors the awk the Linux launcher uses (-> GAMEPAD then guidInstance=).
func parseGamepadGUID(out string) string {
	var firstAny, firstPad string
	isPad := false
	for _, line := range strings.Split(out, "\n") {
		// A new device block resets the "is this a pad?" flag from its devType line.
		if strings.Contains(line, "devType=") {
			isPad = strings.Contains(line, "GAMEPAD") || strings.Contains(line, "JOYSTICK")
			continue
		}
		if i := strings.Index(line, "guidInstance="); i >= 0 {
			if g := guidRe.FindString(line[i:]); g != "" {
				if firstAny == "" {
					firstAny = g
				}
				if isPad && firstPad == "" {
					firstPad = g
				}
			}
		}
		// Stop before the DI8DEVCLASS_ALL pass so we never pick a keyboard/mouse that
		// somehow matched; the GAMECTRL pass came first.
		if strings.Contains(line, "DI8DEVCLASS_ALL") && firstAny != "" {
			break
		}
	}
	if firstPad != "" {
		return firstPad
	}
	return firstAny
}

// setPad0 writes the controller GUID into the registry (REG_SZ, no braces, matching the
// thug2-settings.reg format THUG2 reads).
func setPad0(guid string) error {
	out, err := exec.Command("reg", "add", settingsKey, "/v", "pad0", "/t", "REG_SZ", "/d", guid, "/f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg add: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
