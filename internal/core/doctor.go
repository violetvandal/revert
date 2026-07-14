package core

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NoCDExeMD5 is the md5 of the no-CD THUG2.exe the shipped .asi mods (HudFix/GlyphFix/
// KeyboardGrid) hardcode addresses against. If a user's exe differs, those mods would
// bind to the wrong offsets, so doctor/build verify it. The pristine US exe already
// matches this (it is the no-CD exe).
const NoCDExeMD5 = "d464781a2863c833c640f7ff6d377ffe"

// Status is the machine-readable lifecycle state the GUI uses to gate steps. The field
// set matches the bash `status --json` so the GUI is platform-agnostic. On Windows
// "wine" is always true (the game runs natively) and "setup" reflects the native
// prereqs (DirectX 9).
type Status struct {
	Wine     bool `json:"wine"`
	Setup    bool `json:"setup"`
	Thugkit  bool `json:"thugkit"`
	Pristine bool `json:"pristine"`
	Build    bool `json:"build"`
	Online   bool `json:"online"`
}

// ComputeStatus gathers the native lifecycle state (Windows or macOS).
func ComputeStatus(c *Conf) Status {
	if IsMac() {
		return computeStatusMac(c)
	}
	return Status{
		Wine:     true,             // native Windows: no Wine runtime needed
		Setup:    pad0Configured(), // "setup done" = controller bound (DX9 is optional, not a gate)
		Thugkit:  thugkitHasBuild(c.Thugkit()),
		Pristine: dirExists(filepath.Join(c.Path("PRISTINE_DIR"), "Data", "pre")),
		Build:    dirExists(filepath.Join(c.Path("EDITION_QOL"), "Data", "pre")),
		Online:   thugProInstalled(),
	}
}

// StatusJSON runs doctor's checks and prints the status object as JSON on stdout.
func StatusJSON(c *Conf) {
	s := ComputeStatus(c)
	fmt.Printf(`{"wine":%t,"setup":%t,"thugkit":%t,"pristine":%t,"build":%t,"online":%t}`+"\n",
		s.Wine, s.Setup, s.Thugkit, s.Pristine, s.Build, s.Online)
}

// Doctor prints human-readable prerequisite checks. On Linux it delegates to the bash
// dispatcher (whose doctor knows about Wine/prefixes/evdev); on Windows it reports the
// native prerequisites.
func Doctor(c *Conf) error {
	if IsLinux() {
		return DelegateToBash(c.Root, "doctor")
	}
	if IsMac() {
		return doctorMac(c)
	}
	fmt.Println("[revert] Revert doctor (Windows) — checking prerequisites")

	fmt.Println("Runtime:")
	ok("THUG2 runs natively on Windows (no Wine)")
	if directXPresent() {
		ok("DirectX 9 helper (d3dx9) present")
	} else {
		note("DirectX 9 helper (d3dx9) not detected — optional; THUG2 runs on Windows' built-in DirectX. Only install the DirectX End-User Runtime if the game fails to launch.")
	}

	fmt.Println("Build toolchain:")
	if thugkitHasBuild(c.Thugkit()) {
		ok("thugkit.exe has 'build'")
	} else {
		note("thugkit.exe with 'build' not found (" + c.Thugkit() + ")")
	}
	if _, err := exec.LookPath("python"); err == nil {
		ok("python (optional CAS asset steps)")
	} else if _, err := exec.LookPath("python3"); err == nil {
		ok("python3 (optional CAS asset steps)")
	} else {
		note("python absent (panty/sticker/deck CAS steps skipped; core edition still builds)")
	}

	fmt.Println("Game data (you must own THUG2):")
	if dirExists(filepath.Join(c.Path("PRISTINE_DIR"), "Data", "pre")) {
		ok("pristine base (" + c.Path("PRISTINE_DIR") + ")")
	} else {
		note("no pristine base yet (run: revert acquire-game-data --from <your THUG2 folder>)")
	}
	nocd := c.Path("NOCD_EXE")
	if fileExists(nocd) {
		if sum, err := fileMD5(nocd); err == nil && sum == NoCDExeMD5 {
			ok("no-CD THUG2.exe present (md5 matches the .asi mods)")
		} else if err == nil {
			bad("no-CD THUG2.exe md5 " + sum + " != expected " + NoCDExeMD5 + " (HudFix/GlyphFix/KeyboardGrid may misbehave)")
		}
	} else {
		note("no-CD THUG2.exe not present (user-supplied): " + nocd)
	}
	if fileExists(c.Path("WSFIX_ZIP")) {
		ok("WidescreenFix zip present")
	} else {
		note("WidescreenFix zip not present (user-supplied): " + c.Path("WSFIX_ZIP"))
	}
	return nil
}

// thugkitHasBuild reports whether the thugkit binary exists and exposes 'build'.
func thugkitHasBuild(path string) bool {
	if !fileExists(path) {
		return false
	}
	out, _ := exec.Command(path).CombinedOutput() // usage text lists subcommands
	return strings.Contains(string(out), "build")
}

// fileMD5 returns the lowercase hex md5 of a file.
func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ok(msg string)   { fmt.Printf("  ✓ %s\n", msg) }
func bad(msg string)  { fmt.Printf("  ✗ %s\n", msg) }
func note(msg string) { fmt.Printf("  · %s\n", msg) }
