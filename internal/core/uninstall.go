package core

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Uninstall removes what Revert installed. Two depths:
//
//	default  — remove the toolkit, the builds, the shortcuts and the registry keys,
//	           after exporting every save + created tag to a dated backup folder.
//	--purge  — additionally delete the saves (no backup), THUG Pro, and (on Linux)
//	           the bootstrap Go toolchain and the system packages setup installed.
//
// Nothing outside the toolkit root is touched unless it is on an explicit allowlist,
// and the plan is printed and confirmed before a single byte is removed.
//
// On Linux/Steam Deck this delegates to share/setup/revert-uninstall.sh, which owns the
// Wine prefixes, the Steam shortcut and the package manifest. Windows is native.
func Uninstall(c *Conf, o UninstallOptions) error {
	if !IsWindows() {
		var args []string
		if o.DryRun {
			args = append(args, "--dry-run")
		}
		if o.Yes {
			args = append(args, "--yes")
		}
		if o.Purge {
			args = append(args, "--purge")
		}
		return DelegateToBash(c.Root, "uninstall", args...)
	}
	return uninstallWindows(c, o)
}

// UninstallOptions mirror `revert uninstall`.
type UninstallOptions struct {
	DryRun bool // print the plan, change nothing
	Yes    bool // skip the interactive confirmation
	Purge  bool // full clean: also take saves, THUG Pro, Go, system packages
}

// The Windows shortcut name, duplicated from installdesktop.go's local const so the two
// can't silently drift apart: a rename there without one here would orphan a shortcut.
const shortcutFileName = "THUG2 Violet Vandal Edition.lnk"

// thug2RegKey is the game's whole registry subtree (Settings, pad0, the k0_0 map). The
// parent of settingsKey, which is what `reg delete` needs to take the lot.
const thug2RegKey = `HKCU\Software\Activision\Tony Hawk's Underground 2`

// item is one thing to remove (or export), with the label the plan prints for it.
type item struct {
	Path  string
	Label string
}

// uninstallPlan is the complete, reviewed-before-execution description of the removal.
// Building it never touches the disk beyond stat-ing, so --dry-run is exactly the same
// code path as the real thing, minus execute().
type uninstallPlan struct {
	Purge     bool
	BackupDir string // "" when purging (saves are deleted, not exported)
	Exports   []item // copied into BackupDir before anything is removed
	Paths     []item // removed in order; the toolkit root comes last
	RegKeys   []string
	Notes     []string // what we are deliberately NOT removing, and why
}

func uninstallWindows(c *Conf, o UninstallOptions) error {
	p, err := buildWindowsPlan(c, o.Purge)
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

// buildWindowsPlan surveys the install and returns everything that should go. Every path
// it emits has already passed the allowlist check, so execute() can trust the plan.
func buildWindowsPlan(c *Conf, purge bool) (*uninstallPlan, error) {
	if err := checkRootSane(c.Root); err != nil {
		return nil, err
	}
	p := &uninstallPlan{Purge: purge}
	allow := allowedOutsideRoot(c, purge)

	// A path is only ever planned through here: inside the root, or explicitly allowed.
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

	// ── saves: export first, or delete outright under --purge ──
	saveDirs := []item{
		{Path: filepath.Join(c.Path("EDITION_QOL"), "Save"), Label: "QOL-Modded saves"},
		{Path: filepath.Join(c.Path("EDITION_VANILLA"), "Save"), Label: "Vanilla-edition saves"},
		// The Windows vanilla lane runs the pristine base directly, so saves land here too.
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

	// ── the toolkit root, child by child ──
	// Enumerating children (rather than the root in one shot) is what lets us delete
	// everything around the running revert.exe, which Windows keeps locked.
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		return nil, fmt.Errorf("reading the toolkit root %s: %w", c.Root, err)
	}
	for _, e := range entries {
		add(&p.Paths, filepath.Join(c.Root, e.Name()), rootChildLabel(e.Name()))
	}

	// ── outside the root ──
	for _, lnk := range windowsShortcutPaths() {
		add(&p.Paths, lnk, "shortcut")
	}
	for _, tmp := range staleUpdateDirs() {
		add(&p.Paths, tmp, "leftover update scratch")
	}
	if purge {
		add(&p.Paths, thugProDir(), "THUG Pro (online lane)")
	} else if dirOrFileExists(thugProDir()) {
		p.Notes = append(p.Notes, "keeping THUG Pro ("+thugProDir()+") — it is a separate community app; --purge removes it")
	}

	// Last, so its children (and the running revert.exe among them) are dealt with first.
	add(&p.Paths, c.Root, "the toolkit folder itself")

	p.RegKeys = append(p.RegKeys, thug2RegKey)

	// DirectX 9 is a shared Microsoft runtime that other games link against, and it has no
	// clean per-app uninstall. Removing it would be both antisocial and unsupported, so it
	// stays even under --purge.
	if directXPresent() {
		p.Notes = append(p.Notes, "keeping the DirectX 9 runtime — it is a shared Microsoft component other games use")
	}
	return p, nil
}

// rootChildLabel gives the plan a human line for each top-level entry, so a user reading
// the preview can see that the multi-gigabyte game folders are what's actually going.
func rootChildLabel(name string) string {
	switch {
	case strings.HasPrefix(name, "game-"):
		return "game data / built edition"
	case name == ".revert-cache":
		return "build cache"
	case strings.HasSuffix(name, ".exe"):
		return "toolkit binary"
	default:
		return "toolkit files"
	}
}

// ── safety ──────────────────────────────────────────────────────────────────────

// checkRootSane refuses to proceed against a root that would make the removal catastrophic
// if some config value were empty or wrong. A misparsed revert.conf must never end with us
// walking the user's home directory.
func checkRootSane(root string) error {
	if root == "" {
		return fmt.Errorf("no toolkit root resolved — refusing to uninstall")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolving the toolkit root: %w", err)
	}
	abs = filepath.Clean(abs)
	if abs == filepath.Dir(abs) {
		return fmt.Errorf("the toolkit root resolved to the filesystem root (%s) — refusing to uninstall", abs)
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == abs {
		return fmt.Errorf("the toolkit root resolved to your home directory (%s) — refusing to uninstall", abs)
	}
	if !fileExists(filepath.Join(abs, "revert.conf")) {
		return fmt.Errorf("%s does not look like a Revert install (no revert.conf) — refusing to uninstall", abs)
	}
	return nil
}

// allowedOutsideRoot lists the exact locations outside the toolkit that uninstall may
// remove. Anything not inside the root and not equal-to-or-inside one of these is refused.
func allowedOutsideRoot(c *Conf, purge bool) []string {
	var out []string
	out = append(out, windowsShortcutPaths()...)
	out = append(out, staleUpdateDirs()...)
	if purge {
		if d := thugProDir(); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// removable reports whether path may be deleted: it is the root, inside the root, or
// equal-to-or-inside one of the allowlisted absolute locations.
func removable(root string, allow []string, path string) bool {
	if path == "" {
		return false
	}
	if withinDir(root, path) {
		return true
	}
	for _, a := range allow {
		if a != "" && withinDir(a, path) {
			return true
		}
	}
	return false
}

func windowsShortcutPaths() []string {
	var out []string
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		out = append(out, filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs", shortcutFileName))
	}
	if up := os.Getenv("USERPROFILE"); up != "" {
		out = append(out, filepath.Join(up, "Desktop", shortcutFileName))
	}
	return out
}

// staleUpdateDirs finds scratch dirs the updater left behind (os.MkdirTemp "revert-update-").
func staleUpdateDirs() []string {
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "revert-update-*"))
	if err != nil {
		return nil
	}
	return matches
}

// ── plan output + execution ─────────────────────────────────────────────────────

func (p *uninstallPlan) print() {
	if p.Purge {
		fmt.Println("[revert] uninstall --purge — FULL CLEAN. This will remove:")
	} else {
		fmt.Println("[revert] uninstall — this will remove:")
	}
	for _, it := range p.Paths {
		fmt.Printf("  ✗ %s\n      %s\n", it.Label, it.Path)
	}
	for _, k := range p.RegKeys {
		fmt.Printf("  ✗ registry key\n      %s\n", k)
	}
	if p.BackupDir != "" && len(p.Exports) > 0 {
		fmt.Println("\n  Your saves are backed up first, to:")
		fmt.Printf("      %s\n", p.BackupDir)
		for _, e := range p.Exports {
			fmt.Printf("        · %s\n", e.Label)
		}
	}
	for _, n := range p.Notes {
		fmt.Printf("  · %s\n", n)
	}
}

func (p *uninstallPlan) execute() error {
	// Saves go first. If the export fails we stop, having removed nothing.
	if p.BackupDir != "" {
		for _, e := range p.Exports {
			dst := filepath.Join(p.BackupDir, filepath.Base(filepath.Dir(e.Path)), filepath.Base(e.Path))
			if err := exportPath(e.Path, dst); err != nil {
				return fmt.Errorf("backing up %s: %w — nothing has been removed", e.Label, err)
			}
		}
		if len(p.Exports) > 0 {
			ok("saves backed up to " + p.BackupDir)
		}
	}

	for _, k := range p.RegKeys {
		if err := deleteRegKey(k); err != nil {
			note("registry key not removed (" + k + "): " + err.Error())
		} else {
			ok("registry keys removed")
		}
	}

	// A running .exe cannot be deleted on Windows. Everything around it goes; what's left
	// is reported so the user can drop the folder in one drag.
	var locked []string
	for _, it := range p.Paths {
		if err := os.RemoveAll(it.Path); err != nil {
			locked = append(locked, it.Path)
		}
	}

	fmt.Println()
	if len(locked) == 0 {
		ok("uninstall complete — nothing left behind.")
	} else {
		ok("uninstall complete.")
		fmt.Println("\n[revert] These files are still in use by the program you are running right now,")
		fmt.Println("         so Windows would not let them be deleted:")
		for _, l := range locked {
			fmt.Printf("           %s\n", l)
		}
		fmt.Println("\n         Close this window, then delete that folder to finish.")
	}
	if p.BackupDir != "" && len(p.Exports) > 0 {
		fmt.Printf("\n[revert] Your saves are safe at: %s\n", p.BackupDir)
	}
	return nil
}

// deleteRegKey removes a registry subtree. `reg delete` exits non-zero when the key is
// already absent, which is a success for us, so that case is filtered out.
func deleteRegKey(key string) error {
	out, err := exec.Command("reg", "delete", key, "/f").CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(string(out)), "unable to find") {
		return nil
	}
	return fmt.Errorf("reg delete: %v (%s)", err, strings.TrimSpace(string(out)))
}

// exportPath copies a file or a whole directory to dst.
func exportPath(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return copyTree(src, dst)
	}
	return copyFile(src, dst)
}

// uniqueBackupDir returns ~/thug2-saves-backup-<date>, suffixed if that already exists so
// a second uninstall can never overwrite the first one's saves.
func uniqueBackupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving your home directory for the save backup: %w", err)
	}
	base := filepath.Join(home, "thug2-saves-backup-"+time.Now().Format("2006-01-02"))
	cand := base
	for i := 2; dirOrFileExists(cand); i++ {
		cand = fmt.Sprintf("%s-%d", base, i)
	}
	return cand, nil
}

func confirmUninstall(purge bool) error {
	word := "yes"
	if purge {
		word = "PURGE"
		fmt.Print("\nThis is a FULL CLEAN and your saves will NOT be kept.\n")
	}
	fmt.Printf("\nType %s to continue (anything else cancels): ", word)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return fmt.Errorf("cancelled")
	}
	if strings.TrimSpace(line) != word {
		return fmt.Errorf("cancelled")
	}
	return nil
}

func dirOrFileExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}
