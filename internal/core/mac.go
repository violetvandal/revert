package core

// The macOS lane: THUG2 (a 32-bit Direct3D 9 game from 2004) on Apple Silicon.
//
// The runtime stack, bottom to top:
//
//	Rosetta 2      x86 -> ARM, including the special 32-bit mode Wine-likes rely on
//	wine-stable    wow64, so one binary runs the 32-bit PE (Homebrew cask)
//	patched DXVK   our d3d9.dll: stock DXVK hard-requires geometryShader at device
//	               creation and Metal has no geometry-shader stage at all, so every
//	               released DXVK fails vkCreateDevice on an Apple GPU. Ours asks for
//	               the feature only if present. tools/dxvk-mac/ has the patch.
//	MoltenVK       Vulkan -> Metal
//
// Two details are load-bearing and cost us whole sessions to find, so they are enforced
// here rather than left to documentation:
//
//   - The VIRTUAL DESKTOP is mandatory. THUG2 asks for a 640x480 exclusive-fullscreen
//     mode change, winemac cannot do it, and the game exits. Inside `explorer /desktop`
//     the mode change becomes a window resize.
//   - The pad bridge must live INSIDE that desktop. SendInput is wine-desktop-scoped, so
//     a bridge started on the default desktop injects keys nowhere. vv-run.bat starts the
//     bridge and the game from one batch file so they share a desktop process tree.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// macWineDefault is where the Homebrew `wine-stable` cask puts the wow64 wine binary.
	// Still honoured if a user already has it, but no longer how we install Wine — see
	// macEnsureWine.
	macWineDefault = "/Applications/Wine Stable.app/Contents/Resources/wine/bin/wine"
	// macWineCask is the engine. wine-stable (11.0) is the proven one; wine@devel was
	// tried and is less stable here (cold-start crashes, a menu hang).
	macWineCask = "wine-stable"
	// macWineApp is the app bundle inside the upstream tarball.
	macWineApp = "Wine Stable.app"
	// macWineRelURL/SHA pin the exact Wine build the lane is proven on.
	//
	// We fetch this tarball OURSELVES rather than going through Homebrew, because
	// Homebrew is REMOVING every Wine cask (wine-stable, wine@devel, wine@staging) on
	// 2026-09-01: its maintainers no longer accept software that fails Apple's Gatekeeper
	// notarization check. That is a packaging-policy decision, not a Wine problem — the
	// binaries are fine, and the cask was only ever a thin wrapper around this very URL
	// (verify: `brew info --json=v2 --cask wine-stable`).
	//
	// So the deadline simply stops applying to us: same bytes, same checksum Homebrew
	// itself published, fetched directly. This is the pattern install.sh already uses for
	// the Go toolchain, and it makes the lane self-contained (Homebrew becomes optional).
	macWineRelURL = "https://github.com/Gcenx/macOS_Wine_builds/releases/download/11.0_1/wine-stable-11.0_1-osx64.tar.xz"
	macWineSHA256 = "b50dc50ec7f41d58b115a6b685d4d1315ba3c797bd3aa0f49213f2703cb82388"
	// macDesktop names the wine virtual desktop the game and its pad bridge share.
	macDesktop = "thug2"
	// macFallbackRes is used only if the display cannot be probed. 16:10, matching the
	// panel most Apple laptops have, so the widescreen fix does not letterbox.
	macFallbackRes = "1440x900"
	// macDLLOverrides: our patched d3d9, the Ultimate ASI Loader (winmm) that loads the
	// WidescreenFix .asi, and our left-stick de-inverter (dinput8). mscoree/mshtml are
	// blanked to suppress the Mono/Gecko install dialogs.
	macDLLOverrides = "mscoree,mshtml=;d3d9=n,b;winmm=n,b;dinput8=n,b"
)

// macPaths is every macOS-lane location, resolved once from revert.conf (+ defaults).
type macPaths struct {
	Wine       string // the wine binary
	Wineserver string // its wineserver, for clearing stale locks
	WineDir    string // where WE install Wine (no Homebrew needed)
	Prefix     string // the wine prefix
	Syswow64   string // <prefix>/drive_c/windows/syswow64 — where the 32-bit DLLs live
	DXVKDLL    string // our patched d3d9.dll (in-repo)
	DXVKConf   string // the shipping dxvk.conf (in-repo)
	DXVKSeed   string // a pre-warmed shader cache (in-repo), copied in on first setup
	CacheDir   string // where DXVK's state cache + logs live, OUTSIDE the game dir
	Dinput8    string // our dinput8.dll proxy (in-repo)
	RunBat     string // vv-run.bat (in-repo)
	PadBridge  string // vv-padbridge.exe, built at setup
	Reg        string // the known-good controller binding (in-repo)
}

func macResolve(c *Conf) macPaths {
	cache := filepath.Join(c.Root, ".revert-cache", "mac")
	prefix := c.GetOr("PREFIX_MAC", filepath.Join(os.Getenv("HOME"), ".wine-thug2-ws"))
	wine := macResolveWine(c, cache)
	return macPaths{
		Wine:       wine,
		Wineserver: filepath.Join(filepath.Dir(wine), "wineserver"),
		WineDir:    filepath.Join(cache, "wine"),
		Prefix:     prefix,
		Syswow64:   filepath.Join(prefix, "drive_c", "windows", "syswow64"),
		DXVKDLL:    filepath.Join(c.Root, "tools", "dxvk-mac", "d3d9-dxvk-patched-m1.dll"),
		DXVKConf:   filepath.Join(c.Root, "tools", "dxvk-mac", "dxvk.conf"),
		DXVKSeed:   filepath.Join(c.Root, "tools", "dxvk-mac", "THUG2.dxvk-cache"),
		CacheDir:   cache,
		Dinput8:    filepath.Join(c.Root, "tools", "mac-controller", "dinput8.dll"),
		RunBat:     filepath.Join(c.Root, "tools", "mac-controller", "vv-run.bat"),
		PadBridge:  filepath.Join(cache, "vv-padbridge.exe"),
		Reg:        filepath.Join(c.Root, "tools", "controls", "thug2-settings-mac.reg"),
	}
}

// macEnv is the environment every wine invocation in this lane runs under.
//
// The DXVK config and shader cache are addressed by ENVIRONMENT, not by dropping files
// next to THUG2.exe. That keeps game-pristine-us genuinely pristine when the vanilla lane
// runs out of it, and — the real win — it means the warmed shader cache survives a
// `revert build`, which wipes and re-lays the edition directory. A cache that got deleted
// on every rebuild would silently re-introduce the first-lap-of-every-level stutter.
func (p macPaths) env(extra ...string) []string {
	e := []string{
		"WINEPREFIX=" + p.Prefix,
		"WINEDEBUG=" + macWineDebug(),
		"WINEDLLOVERRIDES=" + macDLLOverrides,
		// MoltenVK warns "Metal does not support disabling primitive restart" on EVERY
		// draw call. Harmless, but it floods the log and the synchronous writes cost real
		// frames. 1 = errors only.
		"MVK_CONFIG_LOG_LEVEL=1",
		"DXVK_CONFIG_FILE=" + p.DXVKConf,
		"DXVK_STATE_CACHE_PATH=" + p.CacheDir,
		"DXVK_LOG_PATH=" + p.CacheDir,
	}
	return append(e, extra...)
}

// macWineDebug picks how loud wine is. The `err` channel stays ON, deliberately.
//
// This used to read WINEDEBUG=fixme-all,err-all,warn-all — and that third setting cost us a
// whole session. `err-all` does not mean "errors only"; in wine's syntax it DISABLES the err
// channel, and err:seh: is how wine reports an unhandled exception, with a backtrace. So when
// the .asi mods were crashing the game under Rosetta, wine was writing the crash out and we
// were throwing it away. It looked like a silent freeze, and we went hunting the render path
// for it.
//
// fixme and warn are the genuinely noisy channels (MoltenVK alone floods them) and stay off.
// err is rare, and it is the only thing that speaks up when the game dies.
//
// An explicit WINEDEBUG in the environment wins, for when you want a deeper trace
// (WINEDEBUG=+seh,+relay).
func macWineDebug() string {
	if v := os.Getenv("WINEDEBUG"); v != "" {
		return v
	}
	return "fixme-all,warn-all"
}

// ── setup ───────────────────────────────────────────────────────────────────────

func setupMac(c *Conf, o SetupOptions) error {
	if o.Online || o.OnlineOnly {
		return fmt.Errorf("the online lane (THUG Pro) is not supported on macOS yet — " +
			"THUG Pro ships a Windows installer and its own launcher; use the Linux or Windows lane for online play")
	}
	p := macResolve(c)
	fmt.Println("[revert] macOS setup — Wine + our patched DXVK")

	macCheckHardware()
	if err := macEnsureWine(c, &p); err != nil {
		return err
	}
	if err := macEnsurePrefix(p); err != nil {
		return err
	}
	if err := macInstallDXVK(p); err != nil {
		return err
	}
	if err := calibrateMac(c, p); err != nil {
		note("controller not set up: " + err.Error())
		note("plug the pad in (XInput mode) and retry with: revert calibrate-controller")
	}
	if err := macBuildTools(c, p); err != nil {
		note("build tools: " + err.Error())
	}
	macSeedShaderCache(p)

	if err := InstallDesktop(c); err != nil {
		note("app bundle not created: " + err.Error())
	}

	fmt.Println("[revert] setup done. Next: revert acquire-game-data --from <your THUG2 folder> ; revert build ; revert run qol")
	return nil
}

// macResolveWine picks the Wine binary, in order of preference:
//
//  1. MAC_WINE, if the user pointed it somewhere real (bring-your-own / an override).
//  2. The copy WE installed, inside the toolkit. This is the normal case.
//  3. A pre-existing Homebrew `wine-stable` in /Applications, so a machine that already
//     has one is not made to download 176MB again.
//
// If none exist it returns the managed path anyway, which is where macEnsureWine will put it.
func macResolveWine(c *Conf, cache string) string {
	managed := filepath.Join(cache, "wine", macWineApp, "Contents", "Resources", "wine", "bin", "wine")
	if w := c.Get("MAC_WINE"); w != "" && fileExists(w) {
		return w
	}
	if fileExists(managed) {
		return managed
	}
	if fileExists(macWineDefault) {
		return macWineDefault
	}
	return managed
}

// macEnsureWine makes sure a usable Wine exists, downloading it into the toolkit if not.
//
// We fetch the upstream tarball directly instead of asking Homebrew, because Homebrew is
// removing every Wine cask on 2026-09-01 (see macWineRelURL). Doing it ourselves means the
// lane keeps working past that date, needs no Homebrew at all, and pins one known-good Wine
// rather than drifting with whatever the cask happens to point at.
func macEnsureWine(c *Conf, p *macPaths) error {
	if fileExists(p.Wine) {
		ok("wine present (" + p.Wine + ")")
		macStripQuarantine(p.Wine)
		return nil
	}

	url := c.GetOr("MAC_WINE_URL", macWineRelURL)
	want := c.GetOr("MAC_WINE_SHA256", macWineSHA256)

	if err := os.MkdirAll(p.WineDir, 0o755); err != nil {
		return err
	}
	tgz := filepath.Join(p.WineDir, "wine.tar.xz")
	fmt.Println("[revert] downloading Wine (~176MB, one time)")
	fmt.Println("         " + url)
	if err := download(url, tgz); err != nil {
		return fmt.Errorf("downloading Wine: %w", err)
	}
	defer os.Remove(tgz)

	// Verify before extracting. This is the same checksum Homebrew publishes for the cask,
	// so we are trusting exactly what the cask trusted — no more, no less.
	if want != "" {
		got, err := sha256File(tgz)
		if err != nil {
			return fmt.Errorf("checksumming the Wine download: %w", err)
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("the Wine download does not match its expected checksum "+
				"(got %s, want %s) — refusing to install it", got, want)
		}
		ok("Wine download verified (sha256)")
	}

	// macOS tar reads .tar.xz natively.
	if err := runInherit(p.WineDir, nil, "tar", "-xf", tgz); err != nil {
		return fmt.Errorf("extracting Wine: %w", err)
	}
	if !fileExists(p.Wine) {
		return fmt.Errorf("Wine did not land where expected (%s) — did the upstream tarball layout change?", p.Wine)
	}
	macStripQuarantine(p.Wine)
	ok("Wine installed into the toolkit (no Homebrew needed)")
	return nil
}

// macStripQuarantine removes com.apple.quarantine from the Wine app bundle. Anything
// downloaded from the internet carries it, and a quarantined wine is killed on sight
// ("Killed: 9"). We own the file we just wrote, so this needs no privileges — but a Wine
// that came from Homebrew might, hence the one escalation.
func macStripQuarantine(wineBin string) {
	app := macAppBundleOf(wineBin)
	if app == "" || !dirExists(app) {
		return
	}
	if err := exec.Command("xattr", "-dr", "com.apple.quarantine", app).Run(); err == nil {
		ok("quarantine cleared on " + filepath.Base(app))
		return
	}
	fmt.Println("[revert] clearing macOS quarantine on Wine (needs your password once)")
	if err := runInherit("", nil, "sudo", "xattr", "-dr", "com.apple.quarantine", app); err != nil {
		note("could not clear quarantine — if wine dies with \"Killed: 9\", run:")
		note("  sudo xattr -dr com.apple.quarantine \"" + app + "\"")
		return
	}
	ok("quarantine cleared on " + filepath.Base(app))
}

// macAppBundleOf walks up from a binary to the .app bundle containing it.
func macAppBundleOf(bin string) string {
	for d := filepath.Dir(bin); d != "/" && d != "." && d != ""; d = filepath.Dir(d) {
		if strings.HasSuffix(d, ".app") {
			return d
		}
	}
	return ""
}

// macEnsurePrefix creates the wine prefix. wine-stable is a wow64 build: one prefix runs
// the 32-bit game, so WINEARCH must NOT be forced to win32 (revert.conf sets that for the
// Linux lane, and honouring it here would produce a prefix wine refuses to use).
func macEnsurePrefix(p macPaths) error {
	if dirExists(filepath.Join(p.Prefix, "drive_c")) {
		ok("wine prefix present (" + p.Prefix + ")")
		return nil
	}
	fmt.Println("[revert] creating the wine prefix: " + p.Prefix)
	env := []string{
		"WINEPREFIX=" + p.Prefix,
		"WINEDLLOVERRIDES=mscoree,mshtml=", // no Mono/Gecko dialogs
		"WINEDEBUG=-all",
	}
	if err := runInherit("", env, p.Wine, "wineboot", "-i"); err != nil {
		return fmt.Errorf("creating the wine prefix: %w", err)
	}
	if !dirExists(p.Syswow64) {
		return fmt.Errorf("the wine prefix came up without %s — is this a wow64 wine?", p.Syswow64)
	}
	ok("wine prefix created")
	return nil
}

// macInstallDXVK drops our patched d3d9.dll into the prefix. This is the piece that makes
// the M1 GPU actually render: stock Wine falls back to wined3d, which is CPU-bound under
// Rosetta and gave a sub-15fps slideshow.
func macInstallDXVK(p macPaths) error {
	if !fileExists(p.DXVKDLL) {
		return fmt.Errorf("the patched DXVK d3d9.dll is missing from the toolkit (%s) — "+
			"a fresh clone should ship it; re-clone or rebuild it per tools/dxvk-mac/README.md", p.DXVKDLL)
	}
	dst := filepath.Join(p.Syswow64, "d3d9.dll")
	if err := copyFile(p.DXVKDLL, dst); err != nil {
		return fmt.Errorf("installing the patched d3d9.dll: %w", err)
	}
	ok("patched DXVK d3d9 installed (GPU-accelerated Direct3D 9)")
	return nil
}

// calibrateMac sets the controller up in two steps, and the order matters: import the
// binding map first, then overwrite pad0 with the GUID probed from THIS machine.
//
// pad0 must be probed, never shipped. THUG2 opens only the device whose DirectInput
// guidInstance equals pad0, and those GUIDs are synthesised by the driver stack rather than
// burned into the pad — wine hands out a different one per device and per prefix. The GUID
// baked into the shipped .reg is therefore just a placeholder that happens to be right on
// the Mac it was captured from; on anyone else's it would mean a silently dead controller.
func calibrateMac(c *Conf, p macPaths) error {
	if err := macImportControllerReg(p); err != nil {
		return err
	}
	ok("controller bindings imported (stick/button map, trick keys)")

	guid, err := macDetectPadGUID(c, p)
	if err != nil {
		note("could not detect the pad's DirectInput GUID: " + err.Error())
		note("  keeping the placeholder pad0 from the shipped bindings — the controller may not")
		note("  drive the skater. Plug the pad in (XInput mode) and run: revert calibrate-controller")
		return nil // not fatal: keyboard play still works
	}
	if err := macSetPad0(p, guid); err != nil {
		return fmt.Errorf("writing pad0: %w", err)
	}
	fmt.Printf("[revert] controller calibrated: pad0 -> %s\n", guid)
	return nil
}

// macImportControllerReg imports the binding map: the stick/button assignment and the k0_*
// numpad keys the trick bridge injects (the trick slots gp0_14..19 stay deliberately
// UNBOUND — the bridge delivers those as keystrokes).
//
// It deliberately does NOT write wine's DirectInput\Joysticks "override" key. The Linux
// lane needs that key; on macOS it does double damage — it shifts the DirectInput object
// order (dead controller) AND hides the pad from wine's XInput, which is exactly what the
// trick bridge reads to see the two triggers as separate axes.
func macImportControllerReg(p macPaths) error {
	if !fileExists(p.Reg) {
		return fmt.Errorf("no macOS controller binding at %s", p.Reg)
	}
	return runInherit("", p.env(), p.Wine, "regedit", "/S", p.Reg)
}

// macDetectPadGUID runs the DirectInput probe under wine and returns the attached pad's
// guidInstance. The probe only enumerates (it never acquires a device), but it is still run
// under a timeout: a wedged probe must not be able to wedge `revert setup`.
func macDetectPadGUID(c *Conf, p macPaths) (string, error) {
	probe := probePath(c)
	if probe == "" {
		return "", fmt.Errorf("the DirectInput probe is missing (expected tools/xinput-probe/dinput_probe_guid.exe)")
	}
	// A bare prefix env: no DLL overrides, so the probe talks to wine's real dinput8 rather
	// than loading our stick-inverting proxy.
	env := []string{"WINEPREFIX=" + p.Prefix, "WINEDEBUG=-all"}

	cmd := exec.Command(p.Wine, probe)
	cmd.Env = append(os.Environ(), env...)
	out, err := runWithTimeout(cmd, 30*time.Second)
	if err != nil && len(out) == 0 {
		return "", fmt.Errorf("running the probe under wine: %w", err)
	}
	guid := parseGamepadGUID(string(out))
	if guid == "" {
		return "", fmt.Errorf("no game controller found — is it plugged in and paired in XInput mode? " +
			"(macOS only exposes Microsoft-VID Xbox pads to wine; see tools/mac-controller/README.md)")
	}
	return guid, nil
}

// macSetPad0 writes the probed GUID into the game's registry inside the wine prefix.
func macSetPad0(p macPaths, guid string) error {
	return runInherit("", p.env(), p.Wine, "reg", "add", settingsKey,
		"/v", "pad0", "/t", "REG_SZ", "/d", guid, "/f")
}

// runWithTimeout runs cmd, capturing its output, and kills it if it outruns d.
func runWithTimeout(cmd *exec.Cmd, d time.Duration) ([]byte, error) {
	var buf strings.Builder
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return []byte(buf.String()), err
	case <-time.After(d):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return []byte(buf.String()), fmt.Errorf("timed out after %s", d)
	}
}

// macEnsureThugkit builds the thugkit build core if it is missing or is not a binary this
// machine can run (a checkout can easily leave a foreign-architecture one behind). On macOS
// a Go toolchain is always present, so this is always recoverable — never a hard error.
func macEnsureThugkit(c *Conf) error {
	if !IsMac() {
		return fmt.Errorf("thugkit is missing and this platform cannot build it here")
	}
	tk := c.Thugkit()
	if thugkitHasBuild(tk) {
		return nil
	}
	goBin := findGo()
	if goBin == "" {
		return fmt.Errorf("Go not found — install it (brew install go) and re-run: revert setup")
	}
	src := filepath.Join(c.Root, "tools", "thugkit")
	if !dirExists(src) {
		return fmt.Errorf("thugkit source missing (%s) — clone with --recursive", src)
	}
	fmt.Println("[revert] building thugkit (the build core)")
	if err := runInherit(src, nil, goBin, "build", "-o", tk, "./cmd/thugkit"); err != nil {
		return fmt.Errorf("building thugkit: %w", err)
	}
	if !thugkitHasBuild(tk) {
		return fmt.Errorf("thugkit built but does not run — wrong architecture?")
	}
	ok("thugkit built")
	return nil
}

// macBuildTools compiles the two Go binaries the lane needs: thugkit (the build core, a
// native darwin/arm64 binary) and vv-padbridge.exe (the XInput trick bridge, a Windows PE
// that runs under wine next to the game).
func macBuildTools(c *Conf, p macPaths) error {
	goBin := findGo()
	if goBin == "" {
		return fmt.Errorf("Go not found — install it (brew install go) and re-run: revert setup")
	}
	if err := os.MkdirAll(p.CacheDir, 0o755); err != nil {
		return err
	}

	if err := macEnsureThugkit(c); err != nil {
		return err
	}

	// The bridge is the SAME source the Windows lane ships. Under wine-mac, once the
	// DirectInput override key is absent, wine's XInput sees the pad and exposes LT and RT
	// as separate axes — DirectInput combines them onto one, so LT+RT cancel and "level
	// out" is unreachable. Hence XInput, hence this exe.
	fmt.Println("[revert] building the controller trick bridge (vv-padbridge.exe)")
	env := []string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"}
	if err := runInherit(c.Root, env, goBin, "build", "-o", p.PadBridge, "./cmd/vv-padbridge"); err != nil {
		return fmt.Errorf("building vv-padbridge.exe: %w", err)
	}
	ok("controller trick bridge built")
	return nil
}

// macSeedShaderCache copies the pre-warmed DXVK state cache in on first setup. DXVK
// compiles each pipeline the first time it is seen; the cache persists them, which is why
// "the second lap of a level is cleaner". Seeding it means a new install starts warm
// instead of hitching its way through the first playthrough.
func macSeedShaderCache(p macPaths) {
	if !fileExists(p.DXVKSeed) {
		return
	}
	dst := filepath.Join(p.CacheDir, "THUG2.dxvk-cache")
	if fileExists(dst) {
		return // the user's own cache is warmer than ours; never clobber it
	}
	if err := os.MkdirAll(p.CacheDir, 0o755); err != nil {
		return
	}
	if copyFile(p.DXVKSeed, dst) == nil {
		ok("pre-warmed shader cache installed (fewer first-run hitches)")
	}
}

// vvMods are the three optional .asi mods, by their build flag name.
var vvMods = []string{"hudfix", "glyphfix", "keyboardgrid"}

// macEnabledVVMods reports which of the VV .asi mods the build should include.
//
// Everywhere but macOS: all of them. They are part of the edition.
//
// On macOS: all of them by default too, now that the freeze is fixed. The earlier default was
// "none" because HudFix (and, milder, GlyphFix) patched live game code from a worker thread,
// which is unsound under Rosetta and froze/crashed the menu; both now patch cold in DllMain and
// verified 8/8 clean with all three loaded. MAC_VV_ASI still selects them individually so any
// future regression can be bisected without recompiling anything:
//
//	MAC_VV_ASI="all"                      the default
//	MAC_VV_ASI="none"                     ship none (WidescreenFix only)
//	MAC_VV_ASI="hudfix"                   one at a time — this is how you bisect
//	MAC_VV_ASI="hudfix,glyphfix"          any combination
//
// "0"/"1" are still accepted, so an older revert.conf.local keeps working.
func macEnabledVVMods(c *Conf) map[string]bool {
	all := map[string]bool{}
	for _, m := range vvMods {
		all[m] = true
	}
	if !IsMac() {
		return all
	}
	switch v := strings.ToLower(strings.TrimSpace(c.GetOr("MAC_VV_ASI", "all"))); v {
	case "", "0", "none", "off", "false":
		return map[string]bool{}
	case "1", "all", "on", "true":
		return all
	default:
		out := map[string]bool{}
		for _, m := range splitCSV(v) {
			if all[m] {
				out[m] = true
			}
		}
		return out
	}
}

// findGo locates the Go toolchain: on PATH, then the local install install.sh makes (it
// deliberately does not touch the user's shell profile), then Homebrew's.
func findGo() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("HOME"), ".local", "go", "bin", "go"),
		"/opt/homebrew/bin/go",
		"/usr/local/bin/go",
	} {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// ── hardware ────────────────────────────────────────────────────────────────────

// macIsAppleSilicon reports whether we are on an M-series Mac (arm64) as opposed to an
// Intel one. Note this reads the CHIP, not the process: even a translated x86 build of
// revert would answer correctly, because it asks sysctl rather than runtime.GOARCH.
func macIsAppleSilicon() bool {
	if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		return strings.Contains(string(out), "Apple")
	}
	return runtime.GOARCH == "arm64"
}

// macCheckHardware reports the chip. The lane is proven on both Apple Silicon and Intel.
//
// Why it works on both:
//   - The patched DXVK d3d9.dll is a 32-bit WINDOWS PE. It is not ARM code and does not
//     care what the host CPU is.
//   - The patch fixes a METAL limitation, not an ARM one: Metal has no geometry-shader
//     stage on ANY Mac GPU, so stock DXVK fails vkCreateDevice on Intel Macs too; ours
//     is needed there just the same.
//   - Intel drops Rosetta entirely (native x86), so it skips this lane's single biggest
//     cost on Apple Silicon.
//
// Validated on a 2018 MacBook Pro (i9-8950HK, Intel UHD 630 + Radeon Pro Vega 20,
// macOS 15): install, boot, render, and play all work out of the box.
func macCheckHardware() {
	if macIsAppleSilicon() {
		ok("Apple Silicon detected (proven here)")
		return
	}
	ok("Intel Mac detected (supported — tested on a 2018 i9 / UHD 630 + Vega 20)")
}

// macBrew locates Homebrew (Apple Silicon first, then Intel, then PATH).
func macBrew() string {
	for _, p := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
		if fileExists(p) {
			return p
		}
	}
	if p, err := exec.LookPath("brew"); err == nil {
		return p
	}
	return ""
}

// ── run ─────────────────────────────────────────────────────────────────────────

func runMac(c *Conf, o RunOptions) error {
	p := macResolve(c)
	if !fileExists(p.Wine) {
		return fmt.Errorf("wine is not installed (%s) — run: revert setup", p.Wine)
	}
	if !dirExists(p.Prefix) {
		return fmt.Errorf("the wine prefix does not exist (%s) — run: revert setup", p.Prefix)
	}

	dir, err := macLaneDir(c, o.Lane)
	if err != nil {
		return err
	}
	exe := c.GetOr("LANE_"+strings.ToUpper(o.Lane)+"_EXE", "THUG2.exe")
	if !fileExists(filepath.Join(dir, exe)) {
		return fmt.Errorf("%s not found in %s", exe, dir)
	}

	res := macResolution(c)
	glyphs := resolveGlyphs(firstNonEmpty(o.Glyphs, c.GetOr("GLYPH_STYLE", "auto")))

	soundtrack := firstNonEmpty(o.Soundtrack, c.GetOr("LANE_"+strings.ToUpper(o.Lane)+"_SOUNDTRACK", "original"))
	if hasHook(splitCSV(c.Get("LANE_"+strings.ToUpper(o.Lane)+"_HOOKS")), "soundtrack") {
		if err := swapSoundtrack(c, dir, soundtrack); err != nil {
			fmt.Printf("[run] (soundtrack swap skipped: %v)\n", err)
		}
	}

	if err := macStageRuntime(p, dir, res); err != nil {
		return err
	}

	fmt.Printf("[run] lane=%s exe=%s %s glyphs=%s\n", o.Lane, exe, res, glyphs)
	fmt.Println("[run] controller: L1/R1 spin · L1+R1 get off · L2/R2 nollie/switch · L2+R2 level out")

	// VV_GLYPH_LIVE=0: on macOS the GlyphFix renderer toggle is applied COLD (boot state only).
	// Re-patching hot code live crashes/freezes THUG2 under Rosetta (the torn-write race the mods
	// were fixed for). A menu change persists and applies on the next launch. Linux/Windows leave
	// this unset and keep the live toggle.
	env := p.env("VV_GLYPHS="+glyphs, "VV_GLYPH_LIVE=0")
	err = macLaunch(p, dir, res, env)

	macCleanup(p)
	macCheckDXVKLog(p)
	if err != nil {
		return err
	}
	fmt.Println("[run] game exited")
	return nil
}

// macLaneDir resolves which directory a lane plays out of.
func macLaneDir(c *Conf, lane string) (string, error) {
	switch lane {
	case "qol":
		d := c.Path("EDITION_QOL")
		if !dirExists(d) {
			return "", fmt.Errorf("the qol edition isn't built yet (%s) — run: revert build", d)
		}
		return d, nil
	case "vanilla":
		// The genuine unmodded original, exactly as the Windows lane does it: the build
		// applies the same mods to every edition, so a built "vanilla" would not be vanilla.
		d := c.Path("PRISTINE_DIR")
		if !dirExists(d) {
			return "", fmt.Errorf("no game data yet (%s) — run: revert acquire-game-data", d)
		}
		return d, nil
	case "online":
		return "", fmt.Errorf("the online lane (THUG Pro) is not supported on macOS yet — use the Linux or Windows lane")
	default:
		return "", fmt.Errorf("unknown lane %q (use: vanilla | qol)", lane)
	}
}

// macStageRuntime puts the per-game-directory pieces in place. It runs on every launch
// because `revert build` re-lays the edition directory and would otherwise strip them.
//
// dinput8_real.dll is the prefix's genuine builtin, copied out under a new name; our proxy
// then LoadLibrary()s it by that name. We only ever READ the prefix's dinput8.dll, never
// overwrite it, so this stays correct however many times setup or run is repeated.
func macStageRuntime(p macPaths, dir, res string) error {
	builtin := filepath.Join(p.Syswow64, "dinput8.dll")
	if !fileExists(builtin) {
		return fmt.Errorf("the wine prefix has no builtin dinput8.dll (%s) — re-run: revert setup", builtin)
	}
	stage := []struct{ src, dst, what string }{
		{p.Dinput8, filepath.Join(dir, "dinput8.dll"), "left-stick de-inverter"},
		{builtin, filepath.Join(dir, "dinput8_real.dll"), "wine's real dinput8"},
		{p.PadBridge, filepath.Join(dir, "vv-padbridge.exe"), "trick bridge"},
		{p.RunBat, filepath.Join(dir, "vv-run.bat"), "one-desktop launcher"},
	}
	for _, s := range stage {
		if !fileExists(s.src) {
			return fmt.Errorf("%s missing (%s) — run: revert setup", s.what, s.src)
		}
		if err := copyFile(s.src, s.dst); err != nil {
			return fmt.Errorf("staging %s: %w", s.what, err)
		}
	}
	macSetWidescreenRes(dir, res)
	return nil
}

// macSetWidescreenRes points the WidescreenFix at the resolution we will actually open the
// virtual desktop at. THUG2 has no in-game resolution option (2004), so the .ini is the
// only place this can be set, and a mismatch between it and the desktop size stretches the
// picture. No-op on the vanilla lane, which has no ASI loader.
func macSetWidescreenRes(dir, res string) {
	ini := filepath.Join(dir, "scripts", "TonyHawksUnderground2.WidescreenFix.ini")
	if !fileExists(ini) {
		return
	}
	w, h, ok2 := splitRes(res)
	if !ok2 {
		return
	}
	b, err := os.ReadFile(ini)
	if err != nil {
		return
	}
	out := regexp.MustCompile(`(?mi)^\s*ResX\s*=.*$`).ReplaceAllString(string(b), "ResX = "+strconv.Itoa(w))
	out = regexp.MustCompile(`(?mi)^\s*ResY\s*=.*$`).ReplaceAllString(out, "ResY = "+strconv.Itoa(h))
	if out != string(b) {
		_ = os.WriteFile(ini, []byte(out), 0o644)
	}
}

// macLaunch runs the game. It pre-warms the wineserver so the launch is never the thing that
// starts it cold (the defence against wine's cold-start race), then runs the game once and
// returns its exit honestly.
//
// It does NOT retry on an early exit. An earlier version relaunched whenever the game exited
// within 20s "to ride out the cold-start race", but that turned a normal quick quit into an
// unwanted second launch — you close the game and it pops straight back up. The pre-warm alone
// has proven sufficient in real installs; if a genuine cold-start failure ever resurfaces,
// re-launch by hand rather than reintroducing the double-launch.
func macLaunch(p macPaths, dir, res string, env []string) error {
	macKillBridge() // a bridge left over from a previous run keeps the desktop alive

	// Pre-warm: start (or keep) the wineserver so the game's launch isn't the cold one.
	if fileExists(p.Wineserver) {
		_ = runInherit("", []string{"WINEPREFIX=" + p.Prefix}, p.Wineserver, "-p")
	}

	args := []string{"explorer", "/desktop=" + macDesktop + "," + res, "cmd", "/c", "vv-run.bat"}
	// Tee the launch so `revert report` has the last run's Wine output to quote. On the
	// Mac lane especially, the interesting failures (MoltenVK, Rosetta, DXVK) all announce
	// themselves on stderr and were previously lost the moment the window closed.
	logw := OpenRunLog("macOS lane, res=" + res)
	err := runTee(logw, dir, env, p.Wine, args...)
	if logw != nil {
		_ = logw.Close()
	}
	return err
}

// macKillBridge reaps the trick bridge. vv-run.bat taskkills it on a clean exit, but if
// the game is force-quit the bridge survives, and its loop keeps the virtual desktop alive
// — which silently blocks the NEXT launch.
func macKillBridge() {
	_ = exec.Command("pkill", "-f", "vv-padbridge").Run()
}

// macCleanup reaps everything a launch leaves behind, and it has to do more than kill the game.
//
// THUG2 exiting does NOT end the wine session: wineserver stays resident, and it keeps
// winedevice/services/explorer alive with it, all holding this prefix open. We used to reap only
// the pad bridge, so those accumulated across launches — every `revert run` left another set
// behind, still mapped, still owning the prefix's lock.
//
// `wineserver -k` terminates every process in the prefix, which is exactly the right hammer here
// and only because the prefix is OURS alone (PREFIX_MAC is dedicated to THUG2). Never point this
// at a prefix a user shares with other Windows apps.
func macCleanup(p macPaths) {
	macKillBridge()
	if fileExists(p.Wineserver) {
		_ = runInherit("", []string{"WINEPREFIX=" + p.Prefix}, p.Wineserver, "-k")
	}
}

// macCheckDXVKLog verifies the config we shipped is the config DXVK actually loaded.
//
// This check exists because we once credited a performance win to a dxvk.conf that had never
// been applied. The log is the only honest witness that our settings reached DXVK at all.
//
// It does NOT claim the config prevents a freeze. It was written believing enableAsync was what
// stopped the menu hanging; that was wrong (see tools/dxvk-mac/dxvk.conf), and the check should
// not go on repeating a diagnosis we have since disproved.
func macCheckDXVKLog(p macPaths) {
	log := filepath.Join(p.CacheDir, "THUG2_d3d9.log")
	b, err := os.ReadFile(log)
	if err != nil {
		// No log at all is the LOUDEST thing this check can find, so it must not be the quietest.
		// It means DXVK never initialised — the game fell back to wined3d, which is CPU-bound
		// under Rosetta and renders a sub-15fps slideshow. Returning silently here meant "totally
		// broken" and "perfectly healthy" produced identical output, which is exactly backwards.
		note("DXVK wrote no log (" + log + ") — it likely did not load at all.")
		note("  without it the game falls back to wined3d, which is a slideshow. Try: revert setup")
		return
	}
	if !strings.Contains(string(b), "enableAsync = True") {
		note("DXVK did not load our config — frame pacing and the shader state cache are off.")
		note("  expected config: " + p.DXVKConf)
	}
}

// ── display ─────────────────────────────────────────────────────────────────────

// macResolution picks the virtual-desktop size: an explicit MAC_RESOLUTION, else the
// display's LOGICAL size (what macOS calls "UI Looks like" — 1440x900 on a 2560x1600
// Retina panel). The logical size is the right target: it is the aspect the panel actually
// presents, and rendering at the full 2x backing resolution would cost frames the Rosetta
// CPU budget cannot spare.
func macResolution(c *Conf) string {
	if r := c.GetOr("MAC_RESOLUTION", "auto"); r != "" && !strings.EqualFold(r, "auto") {
		if _, _, ok2 := splitRes(r); ok2 {
			return r
		}
		note("MAC_RESOLUTION=" + r + " is not WxH — falling back to auto")
	}
	if r := macDetectRes(); r != "" {
		return r
	}
	return macFallbackRes
}

var (
	macJSONResRe     = regexp.MustCompile(`"_spdisplays_resolution"\s*:\s*"(\d+)\s*x\s*(\d+)`)
	macUILooksLikeRe = regexp.MustCompile(`UI Looks like:\s*(\d+)\s*x\s*(\d+)`)
	macResolutionRe  = regexp.MustCompile(`Resolution:\s*(\d+)\s*x\s*(\d+)([^\n]*)`)
)

// macDetectRes asks system_profiler for the display, preferring its JSON output.
//
// The JSON carries a field the human-readable output simply does not have:
// `_spdisplays_resolution` is the LOGICAL size (1440x900), and it is present even over SSH,
// where the text output degrades to the bare physical panel size. Text parsing stays as a
// fallback for any macOS that does not support -json.
func macDetectRes() string {
	if out, err := exec.Command("system_profiler", "SPDisplaysDataType", "-json").Output(); err == nil {
		if r := parseMacDisplayJSON(string(out)); r != "" {
			return r
		}
	}
	out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
	if err != nil {
		return ""
	}
	return parseMacDisplay(string(out))
}

// parseMacDisplayJSON pulls the logical resolution out of `system_profiler -json`.
//
// The three sizes it reports for one Retina panel are all different, and only one of them
// is the right thing to render at:
//
//	spdisplays_pixelresolution  "2560x1600Retina"      the physical panel
//	_spdisplays_pixels          "2880 x 1800"          the backing store macOS renders into
//	_spdisplays_resolution      "1440 x 900 @ 60.00Hz" the LOGICAL size the user sees  <- this
func parseMacDisplayJSON(s string) string {
	if m := macJSONResRe.FindStringSubmatch(s); m != nil {
		return m[1] + "x" + m[2]
	}
	return ""
}

// parseMacDisplay extracts the LOGICAL display size from `system_profiler SPDisplaysDataType`.
//
// Getting this wrong is expensive, not cosmetic: the physical backing store of a Retina
// panel is 4x the pixels of its logical size, and this game is already CPU-bound under
// Rosetta. Opening the virtual desktop at 2560x1600 instead of 1280x800 means rendering
// 3.2x the pixels for no visible gain.
//
// Two shapes come back, and which one you get depends on how the process was launched:
//
//	"UI Looks like: 1440 x 900"        the true logical size, but ONLY present when the
//	                                   process can see the window server (a GUI session).
//	"Resolution: 2560 x 1600 Retina"   always present. This is the PHYSICAL backing store.
//
// Over SSH the first line is simply absent, so trusting "Resolution:" blindly hands wine
// the backing-store size. When it is flagged Retina, halve it: that is the integer-2x
// logical size macOS itself would use, correct aspect, and safe to render.
func parseMacDisplay(s string) string {
	if m := macUILooksLikeRe.FindStringSubmatch(s); m != nil {
		return m[1] + "x" + m[2]
	}
	m := macResolutionRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	w, err1 := strconv.Atoi(m[1])
	h, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return ""
	}
	if strings.Contains(strings.ToLower(m[3]), "retina") {
		w, h = w/2, h/2
	}
	return strconv.Itoa(w) + "x" + strconv.Itoa(h)
}

// splitRes parses "1440x900".
func splitRes(s string) (int, int, bool) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(s)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// ── doctor / status ─────────────────────────────────────────────────────────────

func doctorMac(c *Conf) error {
	p := macResolve(c)
	fmt.Println("[revert] Revert doctor (macOS) — checking prerequisites")

	fmt.Println("Hardware:")
	macCheckHardware()

	fmt.Println("Runtime:")
	if fileExists(p.Wine) {
		ok("wine (" + p.Wine + ")")
	} else {
		bad("wine not installed — run: revert setup")
	}
	if dirExists(p.Prefix) {
		ok("wine prefix (" + p.Prefix + ")")
	} else {
		note("wine prefix not created yet (run: revert setup)")
	}
	if fileExists(filepath.Join(p.Syswow64, "d3d9.dll")) {
		ok("patched DXVK d3d9 installed (GPU-accelerated)")
	} else {
		note("patched DXVK d3d9 not installed (run: revert setup) — without it the game is a slideshow")
	}
	if fileExists(p.DXVKConf) {
		ok("dxvk.conf present (async shader compilation)")
	} else {
		bad("dxvk.conf missing from the toolkit: " + p.DXVKConf)
	}
	fmt.Printf("  · display: virtual desktop %s\n", macResolution(c))

	fmt.Println("Controller:")
	if fileExists(p.Dinput8) {
		ok("left-stick de-inverter proxy present")
	} else {
		bad("dinput8.dll proxy missing from the toolkit: " + p.Dinput8)
	}
	if fileExists(p.PadBridge) {
		ok("trick bridge built")
	} else {
		note("trick bridge not built yet (run: revert setup)")
	}
	note("the pad must identify to macOS as a Microsoft-VID Xbox controller; many third-party")
	note("  XInput pads (8BitDo etc.) are invisible to wine on macOS — see tools/mac-controller/README.md")

	fmt.Println("Build toolchain:")
	if thugkitHasBuild(c.Thugkit()) {
		ok("thugkit has 'build'")
	} else {
		note("thugkit not built yet (run: revert setup)")
	}
	if findGo() != "" {
		ok("Go toolchain")
	} else {
		bad("Go not found (brew install go)")
	}

	fmt.Println("Game data (you must own THUG2):")
	if dirExists(filepath.Join(c.Path("PRISTINE_DIR"), "Data", "pre")) {
		ok("pristine base (" + c.Path("PRISTINE_DIR") + ")")
	} else {
		note("no pristine base yet (run: revert acquire-game-data --from <your THUG2 folder>)")
	}
	if dirExists(filepath.Join(c.Path("EDITION_QOL"), "Data", "pre")) {
		ok("edition built (" + c.Path("EDITION_QOL") + ")")
	} else {
		note("edition not built yet (run: revert build)")
	}
	// The .asi mods hardcode addresses against ONE exe. If it ever differs they bind to the
	// wrong offsets and misbehave, so verify it here as the Windows lane does — this was the
	// first hypothesis for the .asi crash on macOS, and ruling it out took a manual md5.
	if exe := filepath.Join(c.Path("EDITION_QOL"), "THUG2.exe"); fileExists(exe) {
		if sum, err := fileMD5(exe); err == nil {
			if sum == NoCDExeMD5 {
				ok("THUG2.exe md5 matches the one the .asi mods target")
			} else {
				bad("THUG2.exe md5 " + sum + " != " + NoCDExeMD5 + " (the .asi mods would bind to wrong offsets)")
			}
		}
	}
	return nil
}

// computeStatusMac reports the same lifecycle fields the GUI reads on every platform.
func computeStatusMac(c *Conf) Status {
	p := macResolve(c)
	return Status{
		Wine:     fileExists(p.Wine),
		Setup:    dirExists(p.Prefix) && fileExists(filepath.Join(p.Syswow64, "d3d9.dll")),
		Thugkit:  thugkitHasBuild(c.Thugkit()),
		Pristine: dirExists(filepath.Join(c.Path("PRISTINE_DIR"), "Data", "pre")),
		Build:    dirExists(filepath.Join(c.Path("EDITION_QOL"), "Data", "pre")),
		Online:   false, // no THUG Pro on macOS
	}
}

// ── app bundles ─────────────────────────────────────────────────────────────────

// installDesktopMac writes two .app bundles into ~/Applications, so the edition launches
// from Spotlight and the Dock like any other Mac app.
//
// Each bundle's executable is a shell script that execs the revert binary directly — NOT
// the bash dispatcher, which does not run on macOS, and not via `open`, which degrades
// after a few dozen launches (LaunchServices error -1712).
func installDesktopMac(c *Conf) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving the revert binary: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// Install to /Applications like a normal Mac app. On a standard (admin) Mac it is
	// group-writable, so this needs no sudo; fall back to ~/Applications only when we truly
	// can't write there (a non-admin account or a locked-down system).
	apps := "/Applications"
	if !dirWritable(apps) {
		apps = filepath.Join(home, "Applications")
	}
	if err := os.MkdirAll(apps, 0o755); err != nil {
		return err
	}

	// The branded icon, shared with the Windows lane (both derive from tools/pack/icon).
	// Missing is not fatal — the bundle just falls back to the generic app icon.
	icon := filepath.Join(c.Root, "tools", "pack", "icon", "revert.icns")

	bundles := []struct{ name, args, desc string }{
		{"THUG2 Violet Vandal Edition", "run qol", "Play THUG2: Violet Vandal Edition"},
		{"THUG2 Controls", "controls", "Configure the THUG2 controller bindings"},
	}
	made := 0
	for _, b := range bundles {
		if err := writeMacApp(filepath.Join(apps, b.name+".app"), b.name, self, b.args, icon); err != nil {
			note("could not create " + b.name + ".app: " + err.Error())
			continue
		}
		made++
	}
	if made == 0 {
		return fmt.Errorf("no app bundles created")
	}
	ok("app bundles created in " + apps + " (\"THUG2 Violet Vandal Edition\")")
	return nil
}

func writeMacApp(app, name, revertBin, args, iconSrc string) error {
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return err
	}
	// Install the branded icon if we have one. CFBundleIconFile names it WITHOUT the
	// .icns extension, and the file lives in Contents/Resources. If the source is missing
	// we simply omit the key and macOS uses the default app icon.
	iconKey := ""
	if fileExists(iconSrc) {
		res := filepath.Join(app, "Contents", "Resources")
		if err := os.MkdirAll(res, 0o755); err == nil &&
			copyFile(iconSrc, filepath.Join(res, "revert.icns")) == nil {
			iconKey = "  <key>CFBundleIconFile</key><string>revert</string>\n"
		}
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>` + name + `</string>
  <key>CFBundleDisplayName</key><string>` + name + `</string>
  <key>CFBundleIdentifier</key><string>com.violetvandal.` + strings.ReplaceAll(strings.ToLower(name), " ", "-") + `</string>
  <key>CFBundleExecutable</key><string>launch</string>
` + iconKey + `  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>` + Version + `</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}
	sh := "#!/bin/bash\n" +
		"# Generated by `revert install-desktop`. Launches the edition through the revert\n" +
		"# binary, so the lane's wine/DXVK/controller setup is applied exactly as on the CLI.\n" +
		"exec " + shQuote(revertBin) + " " + args + "\n"
	launch := filepath.Join(macos, "launch")
	if err := os.WriteFile(launch, []byte(sh), 0o755); err != nil {
		return err
	}
	return os.Chmod(launch, 0o755)
}

// shQuote single-quotes a string for /bin/sh.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// runMacGUI builds the click-to-install web UI (a separate Go module under gui/) if needed,
// then launches it — the macOS equivalent of the bash dispatcher's cmd_gui. The GUI itself is
// already darwin-aware: it opens the browser with `open` and uses an AppleScript folder picker
// (gui/browser_other.go, gui/main.go). Go is always present on a Mac (it built this binary), so
// a missing gui binary is recoverable rather than fatal.
func runMacGUI(c *Conf, args []string) error {
	guiDir := filepath.Join(c.Root, "gui")
	if !dirExists(guiDir) {
		return fmt.Errorf("the GUI source is missing (%s) — clone with --recursive", guiDir)
	}
	bin := filepath.Join(guiDir, "revert-gui")
	if !fileExists(bin) {
		goBin := findGo()
		if goBin == "" {
			return fmt.Errorf("Go not found — install it (brew install go) to build the GUI, or use the CLI: " +
				"revert setup ; revert build ; revert run qol")
		}
		fmt.Println("[revert] building the GUI (one time)")
		if err := runInherit(guiDir, nil, goBin, "build", "-o", "revert-gui", "."); err != nil {
			return fmt.Errorf("building the GUI: %w", err)
		}
	}
	return runInherit(c.Root, nil, bin, args...)
}

// ── update ────────────────────────────────────────────────────────────────────

// updateMac brings a git-clone install to the latest tagged release and rebuilds. The macOS
// install is a `git clone` made by the installer, so — exactly like the Linux lane — an update
// is fetch → checkout the newest v* tag → rebuild. The one macOS-specific step is rebuilding the
// native Go dispatcher (bin/revert); the Linux dispatcher is bash and needs no build. Game data
// (the pristine base, the built edition, your Save/) is never touched.
func updateMac(c *Conf, o UpdateOptions) error {
	root := c.Root
	git, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is required to update — install Apple's command-line tools: xcode-select --install")
	}
	if !dirExists(filepath.Join(root, ".git")) {
		return fmt.Errorf("not a git checkout (%s) — `revert update` only updates an install made by the installer", root)
	}
	if _, err := gitOut(root, git, "remote", "get-url", "origin"); err != nil {
		return fmt.Errorf("no 'origin' remote — this looks like a local/development checkout; pull and rebuild by hand")
	}

	ulog("fetching releases ...")
	if err := runInherit(root, nil, git, "fetch", "--tags", "--quiet", "origin"); err != nil {
		return fmt.Errorf("git fetch from origin failed — check your network (or run `git fetch origin` by hand): %w", err)
	}

	current, _ := gitOut(root, git, "describe", "--tags", "--always")
	tags, _ := gitOut(root, git, "tag", "-l", "v*", "--sort=-v:refname")
	latest := firstLine(tags)
	if latest == "" {
		return fmt.Errorf("no release tags found on origin")
	}
	tgt, err := gitOut(root, git, "rev-parse", latest+"^{commit}")
	if err != nil {
		return fmt.Errorf("resolving %s: %w", latest, err)
	}
	// Up to date when HEAD already CONTAINS the latest release (at it or ahead of it), so a dev
	// build tracking the branch tip isn't offered a "downgrade".
	if runInherit(root, nil, git, "merge-base", "--is-ancestor", tgt, "HEAD") == nil {
		ulog("already up to date (%s)", current)
		return nil
	}

	ulog("current: %s   latest release: %s", current, latest)
	if o.Check {
		ulog("update available -> run: revert update")
		return nil
	}

	// Preserve local edits to TRACKED files. Machine config belongs in revert.conf.local, which
	// is gitignored and never touched by the checkout.
	stashed := false
	if runInherit(root, nil, git, "diff", "--quiet", "HEAD", "--") != nil {
		if !o.Force {
			return fmt.Errorf("working tree has local changes to tracked files — move config into revert.conf.local, " +
				"commit/stash your edits, or re-run with --force")
		}
		ulog("stashing local changes (restored after update)")
		if runInherit(root, nil, git, "stash", "push", "-u", "-m", "revert-update autostash") == nil {
			stashed = true
		}
	}

	ulog("updating to %s ...", latest)
	if err := runInherit(root, nil, git, "checkout", "-q", latest); err != nil {
		return fmt.Errorf("checkout %s failed: %w", latest, err)
	}
	// No-ops on the flat installer repo (no submodules); real for a --recursive GitHub clone.
	_ = runInherit(root, nil, git, "submodule", "sync", "--recursive", "--quiet")
	_ = runInherit(root, nil, git, "submodule", "update", "--init", "--recursive")
	if stashed {
		if runInherit(root, nil, git, "stash", "pop") != nil {
			uwarn("your stashed changes conflicted — resolve them manually (see: git stash list)")
		}
	}

	goBin := findGo()
	if goBin == "" {
		return fmt.Errorf("Go not found — install it (brew install go) and re-run: revert update")
	}
	// Rebuild the native front door. Overwriting the running executable is safe on macOS: the
	// live process keeps its old inode and finishes; the next `revert` invocation is the new one.
	ulog("rebuilding the revert front door ...")
	if err := runInherit(root, nil, goBin, "build", "-o", filepath.Join("bin", "revert"), "./cmd/revert"); err != nil {
		return fmt.Errorf("rebuilding bin/revert: %w", err)
	}
	// Force a thugkit rebuild — the old binary is stale after a checkout even if it still runs.
	ulog("rebuilding thugkit ...")
	if err := runInherit(filepath.Join(root, "tools", "thugkit"), nil, goBin, "build", "-o", c.Thugkit(), "./cmd/thugkit"); err != nil {
		return fmt.Errorf("rebuilding thugkit: %w", err)
	}

	// Rebuild the edition (idempotent; preserves Save/) via the freshly-built dispatcher.
	if dirExists(filepath.Join(c.Path("PRISTINE_DIR"), "Data", "pre")) {
		ulog("rebuilding the edition ...")
		if err := runInherit(root, nil, filepath.Join(root, "bin", "revert"), "build"); err != nil {
			return fmt.Errorf("revert build failed: %w", err)
		}
	} else {
		ulog("no pristine base yet — run: revert acquire-game-data, then: revert build")
	}

	newver, _ := gitOut(root, git, "describe", "--tags", "--always")
	ulog("updated to %s. Done.", newver)
	return nil
}

// gitOut runs a git command in dir and returns its trimmed stdout.
func gitOut(dir, git string, args ...string) (string, error) {
	cmd := exec.Command(git, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// firstLine returns everything before the first newline.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Controls opens THUG2's own Launcher.exe, the game's controller/graphics config tool. It
// is the escape hatch if a pad ever binds differently from the shipped map.
func Controls(c *Conf) error {
	if !IsMac() {
		return fmt.Errorf("`revert controls` is a macOS command (elsewhere, run THUG2's Launcher.exe from the game folder)")
	}
	p := macResolve(c)
	dir := c.Path("EDITION_QOL")
	if !fileExists(filepath.Join(dir, "Launcher.exe")) {
		dir = c.Path("PRISTINE_DIR")
	}
	if !fileExists(filepath.Join(dir, "Launcher.exe")) {
		return fmt.Errorf("Launcher.exe not found — build the edition first: revert build")
	}
	return runInherit(dir, p.env(), p.Wine, "Launcher.exe")
}

// ── uninstall ───────────────────────────────────────────────────────────────────

func uninstallMac(c *Conf, o UninstallOptions) error {
	p, err := buildMacPlan(c, o.Purge)
	if err != nil {
		return err
	}
	p.print()
	if o.DryRun {
		fmt.Println("\n[revert] dry run — nothing was removed.")
		return nil
	}
	if !o.Yes {
		if err := confirmUninstall(p.Purge); err != nil {
			return err
		}
	}
	return p.execute()
}

// buildMacPlan mirrors buildWindowsPlan: everything Revert created on this Mac, and
// nothing else. The wine prefix holds the game's registry, so there are no separate
// registry keys to delete — removing the prefix takes them.
func buildMacPlan(c *Conf, purge bool) (*uninstallPlan, error) {
	if err := checkRootSane(c.Root); err != nil {
		return nil, err
	}
	mp := macResolve(c)
	p := &uninstallPlan{Purge: purge}
	allow := allowedOutsideRootMac(mp)

	add := func(dst *[]item, path, label string) {
		if path == "" || !dirOrFileExists(path) {
			return
		}
		if !removable(c.Root, allow, path) {
			p.Notes = append(p.Notes, fmt.Sprintf("refused to plan %s (outside the toolkit and not on the allowlist)", path))
			return
		}
		*dst = append(*dst, item{Path: path, Label: label})
	}

	saveDirs := []item{
		{Path: filepath.Join(c.Path("EDITION_QOL"), "Save"), Label: "QOL-Modded saves"},
		{Path: filepath.Join(c.Path("PRISTINE_DIR"), "Save"), Label: "Vanilla (pristine) saves"},
	}
	if local := filepath.Join(c.Root, "revert.conf.local"); fileExists(local) {
		saveDirs = append(saveDirs, item{Path: local, Label: "your local config"})
	}
	if purge {
		p.Notes = append(p.Notes, "saves and created tags will be DELETED, not backed up (--purge)")
	} else {
		backup, err := uniqueBackupDir()
		if err != nil {
			return nil, err
		}
		p.BackupDir = backup
		for _, s := range saveDirs {
			if dirOrFileExists(s.Path) {
				p.Exports = append(p.Exports, s)
			}
		}
	}

	entries, err := os.ReadDir(c.Root)
	if err != nil {
		return nil, fmt.Errorf("reading the toolkit root %s: %w", c.Root, err)
	}
	for _, e := range entries {
		add(&p.Paths, filepath.Join(c.Root, e.Name()), rootChildLabel(e.Name()))
	}

	add(&p.Paths, mp.Prefix, "wine prefix (game settings, controller bindings)")
	for _, app := range macAppPaths() {
		add(&p.Paths, app, "app bundle")
	}
	for _, tmp := range staleUpdateDirs() {
		add(&p.Paths, tmp, "leftover update scratch")
	}

	add(&p.Paths, c.Root, "the toolkit folder itself")

	// Wine: if it is OURS (downloaded into the toolkit) it lives under the root and has
	// already been planned for removal above. If it is the user's — a Homebrew cask, or a
	// build they pointed MAC_WINE at — it is a shared runtime other apps may use, and taking
	// it out from under them would be antisocial. Say so instead, even under --purge.
	if fileExists(mp.Wine) && !withinDir(c.Root, mp.Wine) {
		p.Notes = append(p.Notes, "keeping Wine ("+mp.Wine+") — it is not ours to remove; "+
			"if it came from Homebrew: brew uninstall --cask "+macWineCask)
	}
	return p, nil
}

// allowedOutsideRootMac is the exact set of locations outside the toolkit that uninstall
// may touch on a Mac. Anything else is refused, whatever the config says. Wine itself is
// deliberately absent: it belongs to Homebrew, not to us.
func allowedOutsideRootMac(mp macPaths) []string {
	out := []string{mp.Prefix}
	out = append(out, macAppPaths()...)
	out = append(out, staleUpdateDirs()...)
	return out
}

func macAppPaths() []string {
	// Both locations: bundles now land in /Applications, but an older install (or a non-admin
	// fallback) put them in ~/Applications, so uninstall must clean either.
	dirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	var out []string
	for _, d := range dirs {
		for _, n := range []string{"THUG2 Violet Vandal Edition.app", "THUG2 Controls.app"} {
			out = append(out, filepath.Join(d, n))
		}
	}
	return out
}

// dirWritable reports whether we can actually create files in dir (an existing but
// unwritable dir passes os.Stat but fails here). Used to decide /Applications vs ~/Applications.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".revert-write-test-")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// orNone renders a mod list for humans.
func orNone(s []string) string {
	if len(s) == 0 {
		return "(none)"
	}
	return strings.Join(s, ", ")
}

// vvASIFile maps a mod's flag name to the .asi the ASI loader looks for.
var vvASIFile = map[string]string{
	"hudfix":       "VV.HudFix.asi",
	"glyphfix":     "VV.GlyphFix.asi",
	"keyboardgrid": "VV.KeyboardGrid.asi",
}

// macPruneDisabledASIs deletes any VV .asi the config has turned OFF from the built edition.
//
// Without this, disabling a mod does NOTHING you can observe. thugkit only ever ADDS mods,
// and `revert build --fast` does not re-lay the edition directory — so an .asi installed by
// an earlier build stays there, still loaded by the ASI loader, no matter what the config
// now says. That silently invalidated the first attempt at bisecting the macOS menu freeze:
// the "control" run with MAC_VV_ASI=none still had two mods sitting in scripts/.
//
// Making the config authoritative is also just correct: MAC_VV_ASI should describe what is
// in the game, not what was last added to it.
func macPruneDisabledASIs(edition string, enabled map[string]bool) {
	if !IsMac() {
		return
	}
	for _, mod := range vvMods {
		if enabled[mod] {
			continue
		}
		p := filepath.Join(edition, "scripts", vvASIFile[mod])
		if fileExists(p) && os.Remove(p) == nil {
			note("removed disabled mod: " + vvASIFile[mod])
		}
	}
}
