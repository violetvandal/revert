package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunOptions mirror `revert run <lane> [--soundtrack ..] [--glyphs ..]`.
type RunOptions struct {
	Lane       string // vanilla | qol | online
	Soundtrack string // original | radio (overrides the lane default)
	Glyphs     string // auto | xbox | playstation | gamecube | keyboard
	ExtraArgs  []string
}

// Run launches a lane. On Linux it delegates to the proven share/run/revert-run.sh
// (Wine, prefixes, wineserver lifecycle, the evdev bridges — all validated). On Windows
// the whole Wine apparatus collapses: cd into the edition dir (so the winmm.dll ASI
// loader and the .asi mods resolve), set VV_GLYPHS, optionally swap the soundtrack and
// start the native controller-combo helper, then run THUG2.exe directly.
func Run(c *Conf, o RunOptions) error {
	if !IsWindows() {
		args := []string{o.Lane}
		if o.Soundtrack != "" {
			args = append(args, "--soundtrack", o.Soundtrack)
		}
		if o.Glyphs != "" {
			args = append(args, "--glyphs", o.Glyphs)
		}
		if len(o.ExtraArgs) > 0 {
			args = append(args, "--")
			args = append(args, o.ExtraArgs...)
		}
		return DelegateToBash(c.Root, "run", args...)
	}
	return runWindows(c, o)
}

func runWindows(c *Conf, o RunOptions) error {
	up := strings.ToUpper(o.Lane)
	dir := c.Path("LANE_" + up + "_DIR")
	exe := c.GetOr("LANE_"+up+"_EXE", "")
	hooks := c.Get("LANE_" + up + "_HOOKS")
	soundtrack := o.Soundtrack
	if soundtrack == "" {
		soundtrack = c.GetOr("LANE_"+up+"_SOUNDTRACK", "original")
	}

	// The online lane (THUG Pro) is a native Windows app; its Wine drive_c path in the
	// shared conf is meaningless here. Point at the real install location.
	if o.Lane == "online" {
		if d := thugProDir(); d != "" {
			dir = d
		}
		if exe == "" {
			exe = "THUGProLauncher.exe"
		}
	}
	// Vanilla = the genuine unmodded original. Run the pristine base directly (always present
	// after acquire) instead of a separate modded "vanilla edition" build — the build applies
	// the same mods to every edition, so a built game-modded-vanilla would not be vanilla.
	if o.Lane == "vanilla" {
		if p := c.Path("PRISTINE_DIR"); p != "" {
			dir = p
		}
	}
	if dir == "" || exe == "" {
		return fmt.Errorf("unknown lane %q (use: vanilla | qol | online)", o.Lane)
	}
	if !dirExists(dir) {
		switch o.Lane {
		case "online":
			return fmt.Errorf("THUG Pro isn't installed yet (%s) — run: revert setup --online", dir)
		case "vanilla":
			return fmt.Errorf("no game data yet (%s) — run: revert acquire-game-data", dir)
		default:
			return fmt.Errorf("the %s edition isn't built yet (%s) — run: revert build", o.Lane, dir)
		}
	}

	glyphs := resolveGlyphs(firstNonEmpty(o.Glyphs, c.GetOr("GLYPH_STYLE", "auto")))
	env := []string{"VV_GLYPHS=" + glyphs}
	fmt.Printf("[run] button glyphs -> %s\n", glyphs)

	hookSet := splitCSV(hooks)
	if hasHook(hookSet, "soundtrack") && o.Lane != "online" {
		if err := swapSoundtrack(c, dir, soundtrack); err != nil {
			fmt.Printf("[run] (soundtrack swap skipped: %v)\n", err)
		}
	}

	// padfix: re-detect the controller's DirectInput GUID and refresh pad0 before launch,
	// so a pad on a different port (a new guidInstance) still binds. Best-effort — a missing
	// probe or unplugged pad just leaves pad0 as setup wrote it.
	if hasHook(hookSet, "padfix") && o.Lane != "online" {
		if err := Calibrate(c); err != nil {
			fmt.Printf("[run] (padfix skipped: %v)\n", err)
		}
	}

	// Native controller-combo helper (replaces the Linux evdev bridges). Only the main
	// lanes ask for it; THUG Pro manages its own input.
	var bridge *exec.Cmd
	if (hasHook(hookSet, "trigger-bridge") || hasHook(hookSet, "padfix")) && o.Lane != "online" {
		bridge = startPadBridge(c)
	}

	fmt.Printf("[run] lane=%s exe=%s\n", o.Lane, exe)
	err := runInherit(dir, env, filepath.Join(dir, exe), o.ExtraArgs...)

	if bridge != nil && bridge.Process != nil {
		_ = bridge.Process.Kill()
	}
	code := ExitCode(err)
	fmt.Printf("[run] game exited (code %d)\n", code)
	if err != nil {
		return err
	}
	return nil
}

// startPadBridge launches vv-padbridge.exe (the XInput->keystroke combo helper) as a
// child, or notes it and returns nil if the helper isn't bundled.
func startPadBridge(c *Conf) *exec.Cmd {
	bin := filepath.Join(c.Root, "vv-padbridge.exe")
	if !fileExists(bin) {
		bin2 := filepath.Join(c.Root, "tools", "trigger-bridge", "vv-padbridge.exe")
		if fileExists(bin2) {
			bin = bin2
		} else {
			note("vv-padbridge.exe not bundled — sticks/buttons only (no L2/R2 combo keys)")
			return nil
		}
	}
	cmd := exec.Command(bin)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		note("could not start vv-padbridge: " + err.Error())
		return nil
	}
	fmt.Println("[run] controller combo helper started (LT->KP7 RT->KP9 LB+RB->KP1)")
	return cmd
}

// resolveGlyphs ports revert-run.sh resolve_glyphs (minus the Deck branch): explicit
// style wins; auto -> xbox. Returns one of keyboard|xbox|playstation|gamecube.
func resolveGlyphs(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "keyboard", "xbox", "playstation", "gamecube":
		return strings.ToLower(s)
	case "ps", "ps2":
		return "playstation"
	case "gc", "ngc":
		return "gamecube"
	case "auto", "":
		return "xbox"
	default:
		return "xbox"
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hasHook(hooks []string, name string) bool {
	for _, h := range hooks {
		if h == name {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
