package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildOptions mirror the bash `revert build` flags.
type BuildOptions struct {
	Fast bool
	Lane string // "qol" (default) | "vanilla"
	Only string // comma list passed to thugkit --only
}

// Build produces the edition. On Linux it delegates to the proven bash pipeline; on
// Windows it does the native work itself: shell out to thugkit.exe for the byte-perfect
// core (identical seam to bash), then the optional Python CAS post-pass and credits
// movies. thugkit itself is already 100% cross-platform and byte-identical.
func Build(c *Conf, o BuildOptions) error {
	if IsLinux() {
		args := []string{}
		if o.Fast {
			args = append(args, "--fast")
		}
		if o.Lane != "" && o.Lane != "qol" {
			args = append(args, "--lane", o.Lane)
		}
		if o.Only != "" {
			args = append(args, "--only", o.Only)
		}
		return DelegateToBash(c.Root, "build", args...)
	}

	tk := c.Thugkit()
	if !thugkitHasBuild(tk) {
		// macOS has a Go toolchain (setup guarantees it), so a missing or wrong-architecture
		// thugkit is recoverable — just build it. Erroring out here instead cost real time:
		// a stray Linux binary in the tree kept clobbering the Mac's, and `revert build` then
		// failed in a way that was easy to miss.
		if err := macEnsureThugkit(c); err != nil {
			return fmt.Errorf("thugkit with 'build' not found at %s: %w", tk, err)
		}
	}

	lane := o.Lane
	if lane == "" {
		lane = "qol"
	}
	var edition string
	switch lane {
	case "qol":
		edition = c.Path("EDITION_QOL")
	case "vanilla":
		edition = c.Path("EDITION_VANILLA")
	default:
		return fmt.Errorf("unknown lane %q (use: qol | vanilla)", lane)
	}
	if !dirExists(filepath.Join(c.Path("PRISTINE_DIR"), "Data", "pre")) {
		return fmt.Errorf("no pristine base (%s) — run: revert acquire-game-data", c.Path("PRISTINE_DIR"))
	}

	args := []string{"build", edition, "--pristine", c.Path("PRISTINE_DIR"), "--mods", c.Path("MODS_DIR")}
	if o.Fast {
		args = append(args, "--fast")
	}
	args = appendFlagIfFile(args, "--no-cd", c.Path("NOCD_EXE"))
	args = appendFlagIfFile(args, "--wsfix", c.Path("WSFIX_ZIP"))
	// HQ audio overlay (full builds only) — extract the 7z pack to a cache, excluding the
	// PC dialog pcm.* so it isn't clobbered. Needs 7z on PATH; skip cleanly if absent.
	if !o.Fast {
		if hq := extractHQAudio(c); hq != "" {
			args = append(args, "--hq-audio", hq)
		}
	}
	// The three VV .asi mods (Xbox trick glyphs, top-left HUD, controller text entry) hook
	// the renderer and the input loop. They ship on every platform, macOS included: the menu
	// freeze that used to keep them off the Mac was HudFix/GlyphFix patching live code from a
	// worker thread (a torn write, unsound under Rosetta), and both now patch cold in DllMain.
	// They are still selectable INDIVIDUALLY on macOS (see macEnabledVVMods) so any future
	// regression can be bisected without recompiling.
	vv := macEnabledVVMods(c)
	asi := map[string]string{
		"hudfix":       c.Path("HUDFIX_ASI"),
		"glyphfix":     c.Path("GLYPHFIX_ASI"),
		"keyboardgrid": c.Path("KEYBOARDGRID_ASI"),
	}
	var on, off []string
	for _, m := range vvMods {
		if vv[m] {
			args = appendFlagIfFile(args, "--"+m, asi[m])
			on = append(on, m)
		} else {
			off = append(off, m)
		}
	}
	if IsMac() && len(off) > 0 {
		note("macOS: VV .asi mods — ON: " + orNone(on) + " · OFF: " + orNone(off))
		note("  all three are stable on macOS now; select a subset with MAC_VV_ASI in")
		note("  revert.conf.local:  all | none | a list, e.g. MAC_VV_ASI=\"hudfix\"")
	}
	args = appendFlagIfDir(args, "--tags", c.Path("TAGS_DIR"))
	if o.Only != "" {
		args = append(args, "--only", o.Only)
	}

	fmt.Printf("[revert] build lane=%s%s -> %s\n", lane, fastTag(o.Fast), edition)
	if err := runInherit(c.Root, nil, tk, args...); err != nil {
		return fmt.Errorf("thugkit build failed: %w", err)
	}

	// thugkit only ADDS mods and --fast does not re-lay the edition, so a mod turned off in
	// the config would otherwise linger from a previous build. Make the config authoritative.
	macPruneDisabledASIs(edition, vv)

	casPostPass(c, edition)
	installCreditsMovies(c, edition)
	fmt.Printf("[revert] build done. Play: revert run %s\n", lane)
	return nil
}

// extractHQAudio extracts the HQ audio pack (7z) to .revert-cache/hq-audio, excluding the
// PC dialog pcm.*, and returns the cache dir (or "" if the pack or 7z is unavailable).
func extractHQAudio(c *Conf) string {
	pack := c.Path("HQ_AUDIO_PACK")
	if !fileExists(pack) {
		return ""
	}
	sevenZip := lookPathAny("7z", "7za", "7zr")
	if sevenZip == "" {
		note("7z not found on PATH — skipping HQ audio overlay")
		return ""
	}
	cache := filepath.Join(c.Root, ".revert-cache", "hq-audio")
	if dirExists(filepath.Join(cache, "Game", "Data", "streams")) {
		return cache // already extracted
	}
	fmt.Println("[revert] extracting HQ audio pack (7z, excluding pcm.*)")
	os.MkdirAll(cache, 0o755)
	cmd := exec.Command(sevenZip, "x", "-y", "-o"+cache, pack, "-x!*pcm.wad", "-x!*pcm.dat")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr // 7z chatter to stderr
	if err := cmd.Run(); err != nil {
		note("7z extract failed — skipping HQ audio overlay")
		return ""
	}
	return cache
}

// installCreditsMovies copies the prebuilt in-game credit .bik movies into the build
// (presence-gated; the movies are user/dev-supplied).
func installCreditsMovies(c *Conf, edition string) {
	src := filepath.Join(c.Root, "tools", "bink", "credits")
	dst := filepath.Join(edition, "Data", "movies", "bik")
	if !dirExists(src) || !dirExists(dst) {
		return
	}
	moved := 0
	entries, _ := os.ReadDir(src)
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".bik") {
			if copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())) == nil {
				moved++
			}
		}
	}
	if moved > 0 {
		fmt.Println("[revert] installed credits movies -> Data/movies/bik/")
	}
}

func appendFlagIfFile(args []string, flag, path string) []string {
	if fileExists(path) {
		return append(args, flag, path)
	}
	return args
}

func appendFlagIfDir(args []string, flag, path string) []string {
	if dirExists(path) {
		return append(args, flag, path)
	}
	return args
}

func fastTag(fast bool) string {
	if fast {
		return " (fast)"
	}
	return ""
}

func lookPathAny(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}
