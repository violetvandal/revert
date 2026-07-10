package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const uninstallConf = `EDITION_QOL="${REVERT_ROOT}/game-playable-us"
EDITION_VANILLA="${REVERT_ROOT}/game-modded-vanilla"
PRISTINE_DIR="${REVERT_ROOT}/game-pristine-us"
`

// newInstall lays out a miniature but faithful install: the two built editions with a save
// apiece, the pristine base, some toolkit files, and a "running" revert.exe.
func newInstall(t *testing.T) *Conf {
	t.Helper()
	root := t.TempDir()

	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("revert.conf", uninstallConf)
	write("revert.exe", "MZ")
	write("tools/thugkit/thugkit.exe", "MZ")
	write("game-playable-us/Data/pre/qb_scripts.prx", "prx")
	write("game-playable-us/Save/Violet Vandal.SKA", "save-data")
	write("game-playable-us/Save/VioletVandal.GRF", "tag-data")
	write("game-modded-vanilla/Save/Vanilla.SKA", "vanilla-save")
	write("game-pristine-us/Data/pre/qb_scripts.prx", "prx")

	c, err := LoadConf(filepath.Join(root, "revert.conf"), root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// paths the plan will remove, as a set, for easy assertions.
func plannedPaths(p *uninstallPlan) map[string]bool {
	m := map[string]bool{}
	for _, it := range p.Paths {
		m[it.Path] = true
	}
	return m
}

func TestPlanRemovesTheInstallAndBacksUpSaves(t *testing.T) {
	c := newInstall(t)
	t.Setenv("HOME", t.TempDir())

	p, err := buildWindowsPlan(c, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.BackupDir == "" {
		t.Fatal("a non-purge uninstall must back the saves up somewhere")
	}
	if len(p.Exports) != 2 {
		t.Fatalf("expected both editions' Save dirs exported, got %d: %v", len(p.Exports), p.Exports)
	}

	planned := plannedPaths(p)
	for _, rel := range []string{"game-playable-us", "game-modded-vanilla", "game-pristine-us", "tools", "revert.exe"} {
		if !planned[filepath.Join(c.Root, rel)] {
			t.Errorf("plan does not remove %s", rel)
		}
	}
	if !planned[c.Root] {
		t.Error("plan does not remove the toolkit root itself")
	}
	// The root must be last, so its children (the running revert.exe among them) go first.
	if last := p.Paths[len(p.Paths)-1].Path; last != c.Root {
		t.Errorf("the toolkit root must be removed last, but %s is", last)
	}
	if len(p.RegKeys) != 1 || p.RegKeys[0] != thug2RegKey {
		t.Errorf("expected the THUG2 registry subtree to be planned, got %v", p.RegKeys)
	}
}

func TestPurgeDeletesSavesWithNoBackup(t *testing.T) {
	c := newInstall(t)
	t.Setenv("HOME", t.TempDir())

	p, err := buildWindowsPlan(c, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.BackupDir != "" {
		t.Errorf("purge must not create a backup dir, got %q", p.BackupDir)
	}
	if len(p.Exports) != 0 {
		t.Errorf("purge must export nothing, got %v", p.Exports)
	}
	if !strings.Contains(strings.Join(p.Notes, "\n"), "DELETED") {
		t.Error("purge must warn, in the plan, that saves are deleted")
	}
	// The saves still go, because they live inside the game dirs the plan removes.
	if !plannedPaths(p)[filepath.Join(c.Root, "game-playable-us")] {
		t.Error("purge must still remove the edition (and its saves)")
	}
}

// The one that matters: a plan must never contain a path outside the install.
func TestPlanContainsNothingOutsideTheInstall(t *testing.T) {
	c := newInstall(t)
	t.Setenv("HOME", t.TempDir())

	for _, purge := range []bool{false, true} {
		p, err := buildWindowsPlan(c, purge)
		if err != nil {
			t.Fatal(err)
		}
		allow := allowedOutsideRoot(c, purge)
		for _, it := range p.Paths {
			if !removable(c.Root, allow, it.Path) {
				t.Errorf("purge=%v: plan contains %s, which is outside the install and not allowlisted", purge, it.Path)
			}
		}
	}
}

func TestRemovableRejectsEscapes(t *testing.T) {
	root := "/home/u/thug2"
	allow := []string{"/home/u/.local/share/applications/thug2.desktop"}

	yes := []string{root, root + "/game-playable-us", root + "/a/b/c", allow[0]}
	no := []string{"/", "/home/u", "/home/u/thug2-other", root + "/../evil", "/home/u/.local/share/applications", ""}

	for _, p := range yes {
		if !removable(root, allow, p) {
			t.Errorf("removable(%q) = false, want true", p)
		}
	}
	for _, p := range no {
		if removable(root, allow, p) {
			t.Errorf("removable(%q) = true, want false", p)
		}
	}
}

func TestCheckRootSaneRefusesDangerousRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := checkRootSane(""); err == nil {
		t.Error("an empty root must be refused")
	}
	if err := checkRootSane(string(filepath.Separator)); err == nil {
		t.Error("the filesystem root must be refused")
	}
	if err := checkRootSane(home); err == nil {
		t.Error("the user's home directory must be refused")
	}
	// A plausible-looking dir that isn't a Revert install.
	notAnInstall := t.TempDir()
	if err := checkRootSane(notAnInstall); err == nil {
		t.Error("a directory without revert.conf must be refused")
	}
	// The real thing passes.
	c := newInstall(t)
	if err := checkRootSane(c.Root); err != nil {
		t.Errorf("a real install must be accepted: %v", err)
	}
}

func TestExecuteBacksUpBeforeRemoving(t *testing.T) {
	c := newInstall(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	p, err := buildWindowsPlan(c, false)
	if err != nil {
		t.Fatal(err)
	}
	p.RegKeys = nil // reg.exe is Windows-only; the path removal is what we're exercising

	if err := p.execute(); err != nil {
		t.Fatal(err)
	}

	// Saves survived, byte for byte.
	got, err := os.ReadFile(filepath.Join(p.BackupDir, "game-playable-us", "Save", "Violet Vandal.SKA"))
	if err != nil {
		t.Fatalf("save not exported: %v", err)
	}
	if string(got) != "save-data" {
		t.Errorf("exported save is corrupt: %q", got)
	}
	if _, err := os.Stat(filepath.Join(p.BackupDir, "game-modded-vanilla", "Save", "Vanilla.SKA")); err != nil {
		t.Errorf("vanilla save not exported: %v", err)
	}

	// The install is gone.
	if _, err := os.Stat(c.Root); !os.IsNotExist(err) {
		t.Errorf("the toolkit root survived the uninstall: %v", err)
	}
	// And the backup, which lives outside the root, did not go with it.
	if _, err := os.Stat(p.BackupDir); err != nil {
		t.Errorf("the save backup was destroyed: %v", err)
	}
}

// A failed export must abort before anything is deleted — losing saves is unrecoverable.
func TestExecuteKeepsEverythingIfTheBackupFails(t *testing.T) {
	c := newInstall(t)
	t.Setenv("HOME", t.TempDir())

	p, err := buildWindowsPlan(c, false)
	if err != nil {
		t.Fatal(err)
	}
	p.RegKeys = nil
	// Make the backup destination unusable: a regular file where the directory must go.
	if err := os.WriteFile(p.BackupDir, []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := p.execute(); err == nil {
		t.Fatal("execute must fail when the save backup cannot be written")
	}
	if _, err := os.Stat(filepath.Join(c.Root, "game-playable-us", "Save", "Violet Vandal.SKA")); err != nil {
		t.Errorf("a failed backup must remove nothing, but the save is gone: %v", err)
	}
	if _, err := os.Stat(c.Root); err != nil {
		t.Errorf("a failed backup must remove nothing, but the root is gone: %v", err)
	}
}

func TestUniqueBackupDirNeverOverwritesAnEarlierOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	first, err := uniqueBackupDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := uniqueBackupDir()
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Errorf("a second uninstall on the same day would overwrite the first backup (%s)", first)
	}
}
