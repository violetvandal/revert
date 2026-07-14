package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstLine(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "one": "one", "a\nb": "a", "a\nb\nc": "a", "\nx": "",
	} {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

// updateMac must refuse to touch anything that isn't a proper git-clone install with an
// origin to update from, rather than half-run and leave a broken tree.
func TestUpdateMac_GuardsBeforeTouchingAnything(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	c := &Conf{m: map[string]string{}, Root: root}

	// Not a git checkout at all.
	if err := updateMac(c, UpdateOptions{}); err == nil || !strings.Contains(err.Error(), "not a git checkout") {
		t.Fatalf("want a 'not a git checkout' error, got %v", err)
	}

	// A git checkout, but no 'origin' remote (a local/dev tree) — must not try to fetch.
	if err := exec.Command("git", "-C", root, "init", "-q").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := updateMac(c, UpdateOptions{}); err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("want a 'no origin' error, got %v", err)
	}
}

// A Retina Mac reports BOTH a physical resolution and the logical size macOS actually
// presents. We want the logical one: it is the aspect the panel shows, and rendering the
// 2x backing resolution would cost frames this lane cannot spare.
func TestParseMacDisplay_PrefersLogicalSizeOnRetina(t *testing.T) {
	const retina = `
Displays:
    Color LCD:
      Display Type: Built-in Retina LCD
      Resolution: 2560 x 1600 Retina
      UI Looks like: 1440 x 900 @ 60.00Hz
      Framebuffer Depth: 24-Bit Colour
`
	if got := parseMacDisplay(retina); got != "1440x900" {
		t.Fatalf("Retina panel: got %q, want 1440x900 (the logical size, not the 2560x1600 backing store)", got)
	}
}

// A non-Retina display reports only "Resolution:", and for it that IS the logical size.
func TestParseMacDisplay_FallsBackToResolution(t *testing.T) {
	const plain = `
Displays:
    DELL U2412M:
      Resolution: 1920 x 1200
      Main Display: Yes
`
	if got := parseMacDisplay(plain); got != "1920x1200" {
		t.Fatalf("non-Retina panel: got %q, want 1920x1200", got)
	}
	if got := parseMacDisplay("no displays here"); got != "" {
		t.Fatalf("unparseable output should yield \"\", got %q", got)
	}
}

// Over SSH, system_profiler omits "UI Looks like" entirely and reports only the PHYSICAL
// backing store. Taking that at face value opened the virtual desktop at 2560x1600 instead
// of a logical size — 3.2x the pixels, on a game that is already CPU-bound under Rosetta.
// A Retina-flagged resolution must be halved to its logical size.
func TestParseMacDisplay_HalvesRetinaBackingStore(t *testing.T) {
	const overSSH = `
Graphics/Displays:
    Apple M1:
      Displays:
        Color LCD:
          Display Type: Built-In Retina LCD
          Resolution: 2560 x 1600 Retina
          Main Display: Yes
`
	if got := parseMacDisplay(overSSH); got != "1280x800" {
		t.Fatalf("Retina backing store without a UI-Looks-like line: got %q, want 1280x800 "+
			"(2560x1600 is the 2x backing store, not a resolution to render at)", got)
	}
}

func TestSplitRes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		w, h int
		ok   bool
	}{
		{"1440x900", 1440, 900, true},
		{"1920X1080", 1920, 1080, true},
		{" 1280 x 720 ", 1280, 720, true},
		{"1440*900", 0, 0, false},
		{"widescreen", 0, 0, false},
		{"0x900", 0, 0, false},
		{"-100x900", 0, 0, false},
	} {
		w, h, ok := splitRes(tc.in)
		if ok != tc.ok || w != tc.w || h != tc.h {
			t.Errorf("splitRes(%q) = (%d,%d,%v), want (%d,%d,%v)", tc.in, w, h, ok, tc.w, tc.h, tc.ok)
		}
	}
}

// MAC_RESOLUTION overrides auto-detection, and garbage in it must not produce a garbage
// virtual-desktop size (wine would fail to open the desktop and the game would never start).
func TestMacResolution_ExplicitOverrideAndGarbageFallback(t *testing.T) {
	c := &Conf{m: map[string]string{"MAC_RESOLUTION": "1280x720"}, Root: t.TempDir()}
	if got := macResolution(c); got != "1280x720" {
		t.Fatalf("explicit MAC_RESOLUTION: got %q, want 1280x720", got)
	}

	// Garbage must not be handed to wine. On a non-Mac the probe returns nothing, so this
	// lands on the built-in fallback rather than the bad value.
	c = &Conf{m: map[string]string{"MAC_RESOLUTION": "enormous"}, Root: t.TempDir()}
	if got := macResolution(c); got == "enormous" {
		t.Fatal("a non-WxH MAC_RESOLUTION was passed through to wine verbatim")
	}
}

// The VV .asi mods must be selectable one at a time on macOS — that is what lets us bisect
// which of the three freezes the menu — while NEVER being gated on Linux/Windows, where all
// three ship and are stable.
func TestMacEnabledVVMods(t *testing.T) {
	conf := func(v string) *Conf {
		m := map[string]string{}
		if v != "" {
			m["MAC_VV_ASI"] = v
		}
		return &Conf{m: m, Root: t.TempDir()}
	}
	names := func(c *Conf) []string {
		got := macEnabledVVMods(c)
		var out []string
		for _, m := range vvMods {
			if got[m] {
				out = append(out, m)
			}
		}
		return out
	}

	if !IsMac() {
		// Off-Mac the gate must never fire, whatever MAC_VV_ASI says.
		for _, v := range []string{"", "none", "hudfix"} {
			if len(names(conf(v))) != len(vvMods) {
				t.Errorf("non-Mac with MAC_VV_ASI=%q: all three mods must be built, got %v", v, names(conf(v)))
			}
		}
		return
	}

	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},                      // default: off, the proven-stable config
		{"none", nil},                  //
		{"0", nil},                     // legacy
		{"all", vvMods},                //
		{"1", vvMods},                  // legacy
		{"hudfix", []string{"hudfix"}}, // the bisect case
		{"glyphfix,keyboardgrid", []string{"glyphfix", "keyboardgrid"}},
		{"HudFix", []string{"hudfix"}},       // case-insensitive
		{"hudfix,bogus", []string{"hudfix"}}, // unknown names ignored, not fatal
	} {
		got := names(conf(tc.in))
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("MAC_VV_ASI=%q -> %v, want %v", tc.in, got, tc.want)
		}
	}
}

// macResolve must fall back to working defaults when revert.conf carries no macOS keys, and
// must never point the prefix at the toolkit root (uninstall removes the prefix wholesale).
func TestMacResolve_Defaults(t *testing.T) {
	root := t.TempDir()
	p := macResolve(&Conf{m: map[string]string{}, Root: root})

	// With no Homebrew Wine on the machine, the lane resolves to the copy it manages itself,
	// inside the toolkit. That is what frees us from Homebrew's 2026-09-01 cask removal.
	if !strings.HasPrefix(p.Wine, root) {
		t.Errorf("wine = %q, want it inside the toolkit (%s) when no system Wine exists", p.Wine, root)
	}
	if !strings.HasSuffix(p.Wineserver, "wineserver") {
		t.Errorf("wineserver = %q, want it next to the wine binary", p.Wineserver)
	}
	if p.Prefix == "" || p.Prefix == root {
		t.Fatalf("prefix = %q — must be set and must never be the toolkit root", p.Prefix)
	}
	if !strings.HasPrefix(p.Syswow64, p.Prefix) {
		t.Errorf("syswow64 = %q, want it inside the prefix %q", p.Syswow64, p.Prefix)
	}
	// The shader cache lives OUTSIDE the game directory on purpose: `revert build` re-lays
	// that directory, and a cache stored in it would be destroyed on every rebuild.
	if strings.Contains(p.CacheDir, "game-") {
		t.Errorf("cache dir = %q — must not live inside a game/edition directory", p.CacheDir)
	}
}

// The launch environment carries the three settings that are individually load-bearing.
func TestMacEnv_CarriesLoadBearingSettings(t *testing.T) {
	p := macResolve(&Conf{m: map[string]string{}, Root: t.TempDir()})
	env := strings.Join(p.env("VV_GLYPHS=xbox"), "\n")

	for _, want := range []string{
		"WINEPREFIX=" + p.Prefix,
		"d3d9=n,b",    // our patched DXVK, or the game is a CPU-bound slideshow
		"winmm=n,b",   // the ASI loader, or the widescreen fix never loads
		"dinput8=n,b", // the proxy, or the left stick is inverted
		"DXVK_CONFIG_FILE=" + p.DXVKConf,
		"DXVK_STATE_CACHE_PATH=" + p.CacheDir,
		"VV_GLYPHS=xbox",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("launch env is missing %q\ngot:\n%s", want, env)
		}
	}
}

// A regression guard with real history behind it. The launch env used to carry `err-all`, which
// in wine's syntax DISABLES the err channel — including err:seh:, the unhandled-exception
// backtrace. So the .asi crash that froze the menu under Rosetta was being reported by wine and
// thrown away by us, and it read as a silent hang for an entire session.
//
// Suppressing err is never worth it: the channel is quiet in normal operation, and it is the only
// one that speaks up when the game dies. If someone re-adds it to quieten a log, fail here.
func TestMacWineDebug_NeverSuppressesErr(t *testing.T) {
	t.Setenv("WINEDEBUG", "")

	got := macWineDebug()
	if strings.Contains(got, "err-all") {
		t.Fatalf("WINEDEBUG=%q suppresses the err channel — that hides err:seh: crash backtraces", got)
	}
	// The noisy channels should still be off, or the log is unreadable.
	for _, want := range []string{"fixme-all", "warn-all"} {
		if !strings.Contains(got, want) {
			t.Errorf("WINEDEBUG=%q should still silence %q", got, want)
		}
	}

	// And the whole thing stays overridable, for a deeper trace.
	t.Setenv("WINEDEBUG", "+seh,+relay")
	if got := macWineDebug(); got != "+seh,+relay" {
		t.Errorf("an explicit WINEDEBUG should win, got %q", got)
	}
}

// The uninstall plan is the one piece of this lane that deletes things, so it gets the same
// scrutiny as the Windows one: it must remove the prefix and the app bundles, and it must
// refuse to plan anything outside the toolkit that is not explicitly allowlisted.
func TestBuildMacPlan_RemovesPrefixAndRefusesStrangePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "revert.conf"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(t.TempDir(), ".wine-thug2-ws")
	if err := os.MkdirAll(filepath.Join(prefix, "drive_c"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Something outside the root that must survive: a sibling the user cares about.
	bystander := filepath.Join(t.TempDir(), "important-user-data")
	if err := os.MkdirAll(bystander, 0o755); err != nil {
		t.Fatal(err)
	}

	c := &Conf{m: map[string]string{"PREFIX_MAC": prefix}, Root: root}
	p, err := buildMacPlan(c, false)
	if err != nil {
		t.Fatalf("buildMacPlan: %v", err)
	}

	var planned []string
	for _, it := range p.Paths {
		planned = append(planned, it.Path)
	}
	joined := strings.Join(planned, "\n")

	if !strings.Contains(joined, prefix) {
		t.Errorf("the wine prefix (%s) was not planned for removal; got:\n%s", prefix, joined)
	}
	if strings.Contains(joined, bystander) {
		t.Errorf("planned to remove an unrelated path outside the toolkit: %s", bystander)
	}
	// The toolkit root must be last, so its children (including the running binary) go first.
	if len(p.Paths) == 0 || p.Paths[len(p.Paths)-1].Path != root {
		t.Errorf("the toolkit root must be removed last, got %v", planned)
	}
	// Wine belongs to Homebrew; we only ever tell the user how to remove it.
	if !strings.Contains(strings.Join(p.Notes, " "), "brew uninstall") && fileExists(macWineDefault) {
		t.Error("expected a note explaining that Wine is left alone")
	}
}

// A misparsed config must never let uninstall walk somewhere catastrophic.
func TestBuildMacPlan_RefusesInsaneRoot(t *testing.T) {
	if _, err := buildMacPlan(&Conf{m: map[string]string{}, Root: ""}, false); err == nil {
		t.Error("an empty toolkit root must be refused")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if _, err := buildMacPlan(&Conf{m: map[string]string{}, Root: home}, true); err == nil {
		t.Error("the home directory as toolkit root must be refused")
	}
}

// system_profiler's JSON reports three different sizes for one Retina panel, and only the
// logical one is safe to render at. Picking the wrong field means either a window smaller
// than the screen (the physical/2 fallback) or 4x the pixels (the backing store).
func TestParseMacDisplayJSON_PicksTheLogicalSize(t *testing.T) {
	const j = `{
  "SPDisplaysDataType" : [ { "spdisplays_ndrvs" : [ {
          "_name" : "Color LCD",
          "_spdisplays_pixels" : "2880 x 1800",
          "_spdisplays_resolution" : "1440 x 900 @ 60.00Hz",
          "spdisplays_pixelresolution" : "spdisplays_2560x1600Retina"
  } ] } ]
}`
	if got := parseMacDisplayJSON(j); got != "1440x900" {
		t.Fatalf("got %q, want 1440x900 (not the 2880x1800 backing store, not the 2560x1600 panel)", got)
	}
	if got := parseMacDisplayJSON(`{"nope":1}`); got != "" {
		t.Fatalf("unparseable JSON should yield \"\", got %q", got)
	}
}

// Wine resolution order. This matters because Homebrew is REMOVING every Wine cask on
// 2026-09-01, so the lane must install its own — while still honouring an explicit override
// and not forcing a 176MB re-download on a machine that already has a working Wine.
func TestMacResolveWine_Precedence(t *testing.T) {
	cache := t.TempDir()
	managed := filepath.Join(cache, "wine", macWineApp, "Contents", "Resources", "wine", "bin", "wine")

	// 1. Nothing installed -> the managed path (where macEnsureWine will download it).
	if got := macResolveWine(&Conf{m: map[string]string{}}, cache); got != managed {
		t.Errorf("with no Wine anywhere: got %q, want the managed path %q", got, managed)
	}

	// 2. An explicit MAC_WINE that EXISTS wins (bring-your-own).
	byo := filepath.Join(t.TempDir(), "my-wine")
	if err := os.WriteFile(byo, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Conf{m: map[string]string{"MAC_WINE": byo}}
	if got := macResolveWine(c, cache); got != byo {
		t.Errorf("explicit MAC_WINE: got %q, want %q", got, byo)
	}

	// 3. A MAC_WINE pointing at nothing must NOT be trusted — fall through to the managed
	//    copy rather than handing wine a path that does not exist.
	c = &Conf{m: map[string]string{"MAC_WINE": "/nope/wine"}}
	if got := macResolveWine(c, cache); got != managed {
		t.Errorf("a dangling MAC_WINE should fall through to the managed path, got %q", got)
	}

	// 4. Once we HAVE downloaded Wine, that copy is used.
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := macResolveWine(&Conf{m: map[string]string{}}, cache); got != managed {
		t.Errorf("with a managed Wine present: got %q, want %q", got, managed)
	}
}

// A mod turned OFF must actually leave the game. thugkit only ever ADDS mods and
// `build --fast` does not re-lay the edition directory, so without an explicit prune a
// disabled .asi stays in scripts/ and keeps loading — which silently invalidated the first
// attempt to bisect the macOS menu freeze.
func TestMacPruneDisabledASIs(t *testing.T) {
	if !IsMac() {
		// The prune is macOS-only: Linux/Windows ship all three and must never lose them.
		edition := t.TempDir()
		scripts := filepath.Join(edition, "scripts")
		if err := os.MkdirAll(scripts, 0o755); err != nil {
			t.Fatal(err)
		}
		keep := filepath.Join(scripts, "VV.HudFix.asi")
		if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		macPruneDisabledASIs(edition, map[string]bool{}) // everything "off"
		if !fileExists(keep) {
			t.Error("the prune fired on a non-Mac platform and deleted a shipped mod")
		}
		return
	}

	edition := t.TempDir()
	scripts := filepath.Join(edition, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"VV.HudFix.asi", "VV.GlyphFix.asi", "VV.KeyboardGrid.asi",
		"TonyHawksUnderground2.WidescreenFix.asi"} {
		if err := os.WriteFile(filepath.Join(scripts, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	macPruneDisabledASIs(edition, map[string]bool{"hudfix": true})

	if !fileExists(filepath.Join(scripts, "VV.HudFix.asi")) {
		t.Error("an ENABLED mod was removed")
	}
	for _, gone := range []string{"VV.GlyphFix.asi", "VV.KeyboardGrid.asi"} {
		if fileExists(filepath.Join(scripts, gone)) {
			t.Errorf("%s was disabled but is still in the game — the bisect would be meaningless", gone)
		}
	}
	// WidescreenFix is not ours to touch: it is the one mod the Mac lane always ships.
	if !fileExists(filepath.Join(scripts, "TonyHawksUnderground2.WidescreenFix.asi")) {
		t.Error("the prune removed WidescreenFix, which must always stay")
	}
}
