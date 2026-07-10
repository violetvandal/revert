package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// swapSoundtrack is the Windows Go port of tools/bink/radio/set_soundtrack.sh: a
// launch-time soundtrack switch (a true in-game toggle is engine-impossible — the
// jukebox binds once at cold boot). It swaps the stream .bik files AND the jukebox
// title table before the game starts. Idempotent via a .soundtrack marker.
//
// "original" (the default, shippable path) is fully native Go: restore this build's
// original soundtrack snapshot if one exists, else leave the built (HQ-aware) audio
// untouched. "radio" needs the gitignored royalty-free assets + Python (apply_radio.py)
// and degrades to a noted skip when they're absent. The jukebox title swap shells to
// `thugkit prx replacez` (same seam as the build), skipped if the variant table is
// absent — never blocking launch.
func swapSoundtrack(c *Conf, gameDir, mode string) error {
	music := filepath.Join(gameDir, "Data", "streams", "music")
	orig := filepath.Join(gameDir, "Data", "streams", "music_original")
	state := filepath.Join(gameDir, "Data", "streams", ".soundtrack")
	prx := filepath.Join(gameDir, "Data", "pre", "qb_scripts.prx")
	variants := filepath.Join(c.Root, "tools", "bink", "radio", "variants")
	const entry = "scripts/game/skater/skater_sfx.qb"

	if cur, _ := os.ReadFile(state); strings.TrimSpace(string(cur)) == mode {
		fmt.Printf("[run] soundtrack already %s — no change\n", mode)
		return nil
	}

	switch mode {
	case "radio":
		radioBik := filepath.Join(c.Root, "tools", "bink", "radio", "bik")
		if !dirExists(radioBik) {
			return fmt.Errorf("no radio .bik assets (%s) — keeping the build's soundtrack", radioBik)
		}
		// Snapshot the built (HQ-aware) original once, so we can restore it later.
		if !dirHasFiles(orig) {
			os.MkdirAll(orig, 0o755)
			copyGlob(music, orig, "*.bik")
		}
		py := pythonExe()
		if py == "" {
			return fmt.Errorf("python not found — radio soundtrack needs apply_radio.py")
		}
		tryPy(py, filepath.Join(c.Root, "tools", "bink", "radio", "apply_radio.py"), gameDir)
		swapTitles(c, prx, entry, filepath.Join(variants, "skater_sfx_radio.qb"))
		fmt.Println("[run] soundtrack -> Violet Vandal Radio (royalty-free)")

	case "original":
		if dirHasFiles(orig) {
			copyGlob(orig, music, "*.bik") // restore the built (HQ) original
		}
		os.Remove(filepath.Join(music, "VIOLET_VANDAL_RADIO_credits.txt"))
		swapTitles(c, prx, entry, filepath.Join(variants, "skater_sfx_original.qb"))
		fmt.Println("[run] soundtrack -> Original")

	default:
		return fmt.Errorf("unknown soundtrack %q (use: original | radio)", mode)
	}

	return os.WriteFile(state, []byte(mode+"\n"), 0o644)
}

// swapTitles replaces the jukebox title table (a .qb entry) inside qb_scripts.prx via
// `thugkit prx replacez`. Presence-gated on the variant table (a rebuildable, gitignored
// artifact absent in slim/public clones — the build already carries matching titles, so
// skipping is harmless).
func swapTitles(c *Conf, prx, entry, variant string) {
	if !fileExists(variant) {
		fmt.Printf("[run] (jukebox title table %s not present — keeping the build's titles)\n", filepath.Base(variant))
		return
	}
	tk := c.Thugkit()
	if !fileExists(tk) {
		return
	}
	if err := runInherit(c.Root, nil, tk, "prx", "replacez", prx, entry, variant, prx); err != nil {
		note("jukebox title swap skipped: " + err.Error())
	}
}

func dirHasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// copyGlob copies files matching pattern from src into dst.
func copyGlob(src, dst, pattern string) {
	matches, _ := filepath.Glob(filepath.Join(src, pattern))
	for _, m := range matches {
		_ = copyFile(m, filepath.Join(dst, filepath.Base(m)))
	}
}
