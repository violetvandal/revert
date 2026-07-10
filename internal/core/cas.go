package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// pythonExe finds a Python 3 interpreter on PATH ("" if none). On Windows the launcher
// is usually `python`; `python3` also works if installed from python.org.
func pythonExe() string { return lookPathAny("python", "python3") }

// casPostPass runs the optional Python CAS asset steps that live outside thugkit's Go
// core (texture recolours, licensed deck/model blobs, stickers). Every step is
// presence-gated and best-effort — the core edition is fully playable without any of
// them, and without Python at all. Mirrors the bash cas_post_pass.
func casPostPass(c *Conf, edition string) {
	py := pythonExe()
	if py == "" {
		note("python absent — skipping CAS asset post-pass (core edition still complete)")
		return
	}
	save := filepath.Join(c.Root, "tools", "save")
	fmt.Println("[revert] CAS asset post-pass (python)")

	tryPy(py, filepath.Join(save, "apply_panty_color.py"), edition, "130", "50", "190")
	if d := c.Path("DECKS_BLOB"); dirExists(d) {
		tryPy(py, filepath.Join(save, "apply_deck_pack.py"), edition, d)
	}
	if d := c.Path("PLAYAS_BLOB"); dirExists(d) {
		tryPy(py, filepath.Join(save, "apply_playas_models.py"), edition, d)
	}
	if dirExists(filepath.Join(save, "stickers")) {
		tryPy(py, filepath.Join(save, "apply_stickers.py"), edition)
	}
}

// tryPy runs a python script best-effort; a missing script or a failing step is a note,
// never fatal (the CAS extras are optional polish).
func tryPy(py, script string, args ...string) {
	if !fileExists(script) {
		return
	}
	cmd := exec.Command(py, append([]string{script}, args...)...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		note(filepath.Base(script) + " skipped (" + err.Error() + ")")
	}
}
