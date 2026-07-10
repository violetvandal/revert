package core

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── release metadata ────────────────────────────────────────────────────────────

func TestFetchLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/violetvandal/revert/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("no User-Agent header — GitHub rejects such requests")
		}
		fmt.Fprint(w, `{"tag_name":"v1.4.0","html_url":"https://example/rel",
			"assets":[{"name":"revert-windows-amd64.zip","browser_download_url":"https://example/z","size":1048576}]}`)
	}))
	defer srv.Close()
	defer swapAPIBase(srv.URL)()

	rel, err := fetchLatestRelease("violetvandal/revert")
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "v1.4.0" {
		t.Errorf("TagName = %q, want v1.4.0", rel.TagName)
	}
	a, ok := rel.asset(windowsBundleAsset)
	if !ok {
		t.Fatalf("asset %q not found", windowsBundleAsset)
	}
	if a.Size != 1<<20 {
		t.Errorf("Size = %d, want %d", a.Size, 1<<20)
	}
	if _, ok := rel.asset("nope.zip"); ok {
		t.Error("asset() found an asset that isn't there")
	}
}

func TestFetchLatestReleaseErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"no releases", http.StatusNotFound, "no published releases"},
		{"rate limited", http.StatusForbidden, "rate-limited"},
		{"server error", http.StatusInternalServerError, "HTTP 500"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
			}))
			defer srv.Close()
			defer swapAPIBase(srv.URL)()

			_, err := fetchLatestRelease("violetvandal/revert")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func swapAPIBase(u string) func() {
	orig := githubAPIBase
	githubAPIBase = u
	return func() { githubAPIBase = orig }
}

// ── extraction ──────────────────────────────────────────────────────────────────

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "b.zip")
	writeZip(t, src, map[string]string{
		"revert.exe":                "exe",
		"tools/thugkit/thugkit.exe": "tk",
		"docs/INSTALL.md":           "docs",
	})

	dst := filepath.Join(dir, "stage")
	if err := extractZip(src, dst); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	for name, want := range map[string]string{
		"revert.exe":                "exe",
		"tools/thugkit/thugkit.exe": "tk",
		"docs/INSTALL.md":           "docs",
	} {
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// A malicious archive must not be able to write outside the staging directory.
func TestExtractZipRejectsTraversal(t *testing.T) {
	for _, entry := range []string{"../escaped.txt", "../../escaped.txt", `..\escaped.txt`} {
		t.Run(entry, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "evil.zip")
			writeZip(t, src, map[string]string{entry: "pwned"})

			err := extractZip(src, filepath.Join(dir, "stage"))
			if err == nil || !strings.Contains(err.Error(), "escapes") {
				t.Fatalf("extractZip(%q) error = %v, want an 'escapes' rejection", entry, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "escaped.txt")); err == nil {
				t.Fatal("traversal succeeded: escaped.txt was written outside the staging dir")
			}
		})
	}
}

func TestWithinDir(t *testing.T) {
	root := filepath.FromSlash("/a/b")
	cases := []struct {
		target string
		want   bool
	}{
		{filepath.FromSlash("/a/b"), true},
		{filepath.FromSlash("/a/b/c"), true},
		{filepath.FromSlash("/a/b/c/d.txt"), true},
		{filepath.FromSlash("/a/c"), false},
		{filepath.FromSlash("/a"), false},
		{filepath.FromSlash("/a/bb"), false}, // prefix-but-not-child
	}
	for _, c := range cases {
		if got := withinDir(root, c.target); got != c.want {
			t.Errorf("withinDir(%q, %q) = %v, want %v", root, c.target, got, c.want)
		}
	}
}

// ── verification ────────────────────────────────────────────────────────────────

func TestVerifyBundleChecksum(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, windowsBundleAsset)
	writeZip(t, zipPath, map[string]string{"revert.exe": "payload"})
	sum, err := sha256File(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	serve := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
	}

	t.Run("matching", func(t *testing.T) {
		srv := serve(sum + "  " + windowsBundleAsset + "\n")
		defer srv.Close()
		rel := &ghRelease{Assets: []ghAsset{{Name: windowsBundleAsset + ".sha256", URL: srv.URL}}}
		if err := verifyBundle(rel, zipPath); err != nil {
			t.Errorf("verifyBundle with a good checksum: %v", err)
		}
	})

	t.Run("mismatched", func(t *testing.T) {
		bad := hex.EncodeToString(sha256.New().Sum(nil))
		srv := serve(bad + "  " + windowsBundleAsset + "\n")
		defer srv.Close()
		rel := &ghRelease{Assets: []ghAsset{{Name: windowsBundleAsset + ".sha256", URL: srv.URL}}}
		err := verifyBundle(rel, zipPath)
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("verifyBundle with a bad checksum: err = %v, want a mismatch", err)
		}
	})

	// No sidecar published: still installs, but the zip must at least be readable.
	t.Run("no sidecar", func(t *testing.T) {
		if err := verifyBundle(&ghRelease{}, zipPath); err != nil {
			t.Errorf("verifyBundle without a sidecar: %v", err)
		}
	})

	t.Run("not a zip", func(t *testing.T) {
		junk := filepath.Join(dir, "junk.zip")
		os.WriteFile(junk, []byte("this is not a zip"), 0o644)
		err := verifyBundle(&ghRelease{}, junk)
		if err == nil || !strings.Contains(err.Error(), "readable zip") {
			t.Errorf("verifyBundle on a non-zip: err = %v, want a 'readable zip' rejection", err)
		}
	})
}

// ── install / rollback ──────────────────────────────────────────────────────────

// testConf writes a minimal revert.conf into a temp root and loads it.
func testConf(t *testing.T) *Conf {
	t.Helper()
	root := t.TempDir()
	conf := strings.Join([]string{
		`PRISTINE_DIR="${REVERT_ROOT}/game-pristine-us"`,
		`EDITION_QOL="${REVERT_ROOT}/game-playable-us"`,
		`EDITION_VANILLA="${REVERT_ROOT}/game-modded-vanilla"`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "revert.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConf(filepath.Join(root, "revert.conf"), root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestInstallStagedReplacesAndBacksUp(t *testing.T) {
	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "OLD")

	stage := filepath.Join(t.TempDir(), "stage")
	mustWrite(t, filepath.Join(stage, "revert.exe"), "NEW")
	mustWrite(t, filepath.Join(stage, "tools", "thugkit", "thugkit.exe"), "NEWTK")

	if err := installStaged(c, stage); err != nil {
		t.Fatalf("installStaged: %v", err)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "NEW" {
		t.Errorf("revert.exe = %q, want NEW", got)
	}
	if got := mustRead(t, filepath.Join(c.Root, "tools", "thugkit", "thugkit.exe")); got != "NEWTK" {
		t.Errorf("thugkit.exe = %q, want NEWTK", got)
	}
	// The replaced file was renamed aside, not clobbered — that is what lets a running
	// revert.exe replace itself on Windows.
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe"+backupSuffix)); got != "OLD" {
		t.Errorf("backup = %q, want OLD", got)
	}
}

// An archive that reaches into the game tree must be refused, and anything already written
// must be rolled back.
func TestInstallStagedRefusesProtectedDirsAndRollsBack(t *testing.T) {
	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "aaa.txt"), "ORIGINAL")
	save := filepath.Join(c.Root, "game-playable-us", "Save", "Violet Vandal.SKA")
	mustWrite(t, save, "PRECIOUS")

	// Walk order is lexical: aaa.txt installs before game-playable-us/ is reached.
	stage := filepath.Join(t.TempDir(), "stage")
	mustWrite(t, filepath.Join(stage, "aaa.txt"), "REPLACED")
	mustWrite(t, filepath.Join(stage, "game-playable-us", "Save", "Violet Vandal.SKA"), "EVIL")

	err := installStaged(c, stage)
	if err == nil || !strings.Contains(err.Error(), "protected directory") {
		t.Fatalf("installStaged error = %v, want a protected-directory refusal", err)
	}
	if got := mustRead(t, save); got != "PRECIOUS" {
		t.Errorf("the save file was overwritten: %q", got)
	}
	if got := mustRead(t, filepath.Join(c.Root, "aaa.txt")); got != "ORIGINAL" {
		t.Errorf("rollback failed: aaa.txt = %q, want ORIGINAL", got)
	}
	if fileExists(filepath.Join(c.Root, "aaa.txt"+backupSuffix)) {
		t.Error("rollback left a stray .old backup behind")
	}
}

func TestProtectedDirs(t *testing.T) {
	c := testConf(t)
	got := protectedDirs(c)
	for _, want := range []string{"game-pristine-us", "game-playable-us", "game-modded-vanilla"} {
		found := false
		for _, g := range got {
			if filepath.Base(g) == want {
				found = true
			}
		}
		if !found {
			t.Errorf("protectedDirs() missing %s (got %v)", want, got)
		}
	}
}

func TestSweepBackupsSkipsGameTrees(t *testing.T) {
	root := t.TempDir()
	stray := filepath.Join(root, "revert.exe"+backupSuffix)
	inGame := filepath.Join(root, "game-playable-us", "Data", "thing"+backupSuffix)
	mustWrite(t, stray, "x")
	mustWrite(t, inGame, "x")

	sweepBackups(root)

	if fileExists(stray) {
		t.Error("sweepBackups left a toolkit .old behind")
	}
	// The sweep never descends into the game trees, so a coincidentally-named file there
	// is untouched.
	if !fileExists(inGame) {
		t.Error("sweepBackups deleted a file inside the game tree")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "unknown size", -1: "unknown size", 4096: "4 KB", 18 << 20: "18 MB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────────

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ── end-to-end ──────────────────────────────────────────────────────────────────

// fakeRelease stands up a GitHub-shaped release feed serving `bundle` as the Windows asset
// (plus its checksum sidecar) and points githubAPIBase at it. Returns the cleanup func.
func fakeRelease(t *testing.T, tag string, bundle []byte, withSidecar bool) func() {
	t.Helper()
	sum := sha256.Sum256(bundle)
	mux := http.NewServeMux()
	var base string

	mux.HandleFunc("/zip", func(w http.ResponseWriter, r *http.Request) { w.Write(bundle) })
	mux.HandleFunc("/sha", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), windowsBundleAsset)
	})
	mux.HandleFunc("/repos/violetvandal/revert/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		assets := fmt.Sprintf(`{"name":%q,"browser_download_url":"%s/zip","size":%d}`,
			windowsBundleAsset, base, len(bundle))
		if withSidecar {
			assets += fmt.Sprintf(`,{"name":"%s.sha256","browser_download_url":"%s/sha","size":64}`,
				windowsBundleAsset, base)
		}
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example/rel","assets":[%s]}`, tag, assets)
	})

	srv := httptest.NewServer(mux)
	base = srv.URL
	restore := swapAPIBase(srv.URL)
	return func() { restore(); srv.Close() }
}

// bundleZip builds an in-memory release bundle.
func bundleZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "b.zip")
	writeZip(t, p, entries)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func withVersion(v string) func() {
	orig := Version
	Version = v
	return func() { Version = orig }
}

// The whole flow: check the feed, download, verify the checksum, unpack, swap the binaries
// in place, back up the replaced revert.conf. No game data present, so no rebuild is run.
func TestUpdateWindowsEndToEnd(t *testing.T) {
	defer withVersion("v1.3.0")()

	bundle := bundleZip(t, map[string]string{
		"revert.exe":                "NEW-REVERT",
		"revert.conf":               "PRISTINE_DIR=\"${REVERT_ROOT}/game-pristine-us\"\nNEWKEY=1",
		"tools/thugkit/thugkit.exe": "NEW-THUGKIT",
		"docs/INSTALL.md":           "new docs",
	})
	defer fakeRelease(t, "v1.4.0", bundle, true)()

	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "OLD-REVERT")
	oldConf := mustRead(t, filepath.Join(c.Root, "revert.conf"))

	if err := updateWindows(c, UpdateOptions{}); err != nil {
		t.Fatalf("updateWindows: %v", err)
	}

	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "NEW-REVERT" {
		t.Errorf("revert.exe = %q, want NEW-REVERT", got)
	}
	if got := mustRead(t, filepath.Join(c.Root, "tools", "thugkit", "thugkit.exe")); got != "NEW-THUGKIT" {
		t.Errorf("thugkit.exe = %q, want NEW-THUGKIT", got)
	}
	if got := mustRead(t, filepath.Join(c.Root, "docs", "INSTALL.md")); got != "new docs" {
		t.Errorf("docs not installed: %q", got)
	}
	// revert.conf is toolkit-owned: replaced, but the outgoing copy is kept.
	if got := mustRead(t, filepath.Join(c.Root, "revert.conf")); !strings.Contains(got, "NEWKEY=1") {
		t.Errorf("revert.conf was not replaced: %q", got)
	}
	if got := mustRead(t, filepath.Join(c.Root, confBackup)); got != oldConf {
		t.Errorf("%s = %q, want the outgoing revert.conf", confBackup, got)
	}
	// Nothing is running, so the sweep clears every backup it made.
	if fileExists(filepath.Join(c.Root, "revert.exe"+backupSuffix)) {
		t.Error("a .old backup survived the post-install sweep")
	}
}

// --check reports without touching a single file.
func TestUpdateWindowsCheckIsReadOnly(t *testing.T) {
	defer withVersion("v1.3.0")()
	defer fakeRelease(t, "v1.4.0", bundleZip(t, map[string]string{"revert.exe": "NEW"}), true)()

	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "OLD")

	if err := updateWindows(c, UpdateOptions{Check: true}); err != nil {
		t.Fatalf("updateWindows --check: %v", err)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "OLD" {
		t.Errorf("--check modified revert.exe: %q", got)
	}
	if fileExists(filepath.Join(c.Root, confBackup)) {
		t.Error("--check wrote a conf backup")
	}
}

func TestUpdateWindowsAlreadyUpToDate(t *testing.T) {
	defer withVersion("v1.4.0")()
	defer fakeRelease(t, "v1.4.0", bundleZip(t, map[string]string{"revert.exe": "NEW"}), true)()

	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "OLD")
	if err := updateWindows(c, UpdateOptions{}); err != nil {
		t.Fatalf("updateWindows: %v", err)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "OLD" {
		t.Error("an up-to-date install was overwritten anyway")
	}
}

// A newer local build (ahead of the newest tag) must not be "updated" backwards.
func TestUpdateWindowsRefusesDowngrade(t *testing.T) {
	defer withVersion("v2.0.0")()
	defer fakeRelease(t, "v1.4.0", bundleZip(t, map[string]string{"revert.exe": "OLDER"}), true)()

	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "CURRENT")
	if err := updateWindows(c, UpdateOptions{}); err != nil {
		t.Fatalf("updateWindows: %v", err)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "CURRENT" {
		t.Errorf("downgraded to the older release: %q", got)
	}
}

// An unstamped dev build can't tell newer from older, so it refuses — unless forced.
func TestUpdateWindowsDevBuildRefusesWithoutForce(t *testing.T) {
	defer withVersion(DevVersion)()
	defer fakeRelease(t, "v1.4.0", bundleZip(t, map[string]string{"revert.exe": "NEW"}), true)()

	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "DEV")

	err := updateWindows(c, UpdateOptions{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %v, want a refusal mentioning --force", err)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "DEV" {
		t.Error("a dev build self-updated without --force")
	}

	if err := updateWindows(c, UpdateOptions{Force: true}); err != nil {
		t.Fatalf("updateWindows --force: %v", err)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "NEW" {
		t.Errorf("--force did not install: %q", got)
	}
}

// Today's reality: the published releases carry no Windows asset. The error has to say so
// plainly rather than crash or half-install.
func TestUpdateWindowsNoWindowsAsset(t *testing.T) {
	defer withVersion("v1.3.0")()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v1.3.1","html_url":"https://example/rel",
			"assets":[{"name":"revert-installer-linux-amd64","browser_download_url":"https://example/l","size":7}]}`)
	}))
	defer srv.Close()
	defer swapAPIBase(srv.URL)()

	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "OLD")

	err := updateWindows(c, UpdateOptions{})
	if err == nil || !strings.Contains(err.Error(), windowsBundleAsset) {
		t.Fatalf("error = %v, want it to name the missing %s asset", err, windowsBundleAsset)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "OLD" {
		t.Error("a release with no Windows asset still modified the install")
	}
}

// A corrupted download must abort before anything is swapped.
func TestUpdateWindowsChecksumMismatchAborts(t *testing.T) {
	defer withVersion("v1.3.0")()
	good := bundleZip(t, map[string]string{"revert.exe": "NEW"})

	// Serve a *different* body than the one the sidecar hashes.
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/zip", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("corrupted")) })
	mux.HandleFunc("/sha", func(w http.ResponseWriter, r *http.Request) {
		sum := sha256.Sum256(good)
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), windowsBundleAsset)
	})
	mux.HandleFunc("/repos/violetvandal/revert/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v1.4.0","assets":[
			{"name":%q,"browser_download_url":"%s/zip","size":9},
			{"name":"%s.sha256","browser_download_url":"%s/sha","size":64}]}`,
			windowsBundleAsset, base, windowsBundleAsset, base)
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()
	defer swapAPIBase(srv.URL)()

	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "OLD")

	err := updateWindows(c, UpdateOptions{})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want a checksum mismatch", err)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "OLD" {
		t.Error("a corrupt download was installed")
	}
}

// --check never modifies anything, so on an unstamped build it reports the latest release
// rather than refusing. Only an actual update refuses (see the Force test above).
func TestUpdateWindowsDevBuildCheckReportsInsteadOfFailing(t *testing.T) {
	defer withVersion(DevVersion)()
	defer fakeRelease(t, "v1.4.0", bundleZip(t, map[string]string{"revert.exe": "NEW"}), true)()

	c := testConf(t)
	mustWrite(t, filepath.Join(c.Root, "revert.exe"), "DEV")

	if err := updateWindows(c, UpdateOptions{Check: true}); err != nil {
		t.Fatalf("dev --check should report, not fail: %v", err)
	}
	if got := mustRead(t, filepath.Join(c.Root, "revert.exe")); got != "DEV" {
		t.Error("dev --check modified the install")
	}
}

// ── download progress ────────────────────────────────────────────────────────

func TestProgressWriterReportsAndCounts(t *testing.T) {
	// Pin the clock so rate math is deterministic. Advance 1s per Write so each Write
	// crosses the once-a-second reporting threshold.
	base := time.Unix(1_700_000_000, 0)
	var tick int64
	orig := nowFunc
	nowFunc = func() time.Time { return base.Add(time.Duration(tick) * time.Second) }
	defer func() { nowFunc = orig }()

	var sink bytes.Buffer
	tick = 0
	pw := &progressWriter{w: &sink, total: 30 << 20, start: nowFunc()}
	for i := 0; i < 3; i++ {
		tick++ // +1s before each write triggers a report
		if _, err := pw.Write(make([]byte, 10<<20)); err != nil {
			t.Fatal(err)
		}
	}
	if pw.n != 30<<20 {
		t.Errorf("counted %d bytes, want %d", pw.n, 30<<20)
	}
	if sink.Len() != 30<<20 {
		t.Errorf("underlying writer got %d bytes, want %d", sink.Len(), 30<<20)
	}
	if !pw.reported {
		t.Error("expected at least one progress line to have been reported")
	}
}

// An unknown Content-Length (-1) must not divide by zero or print a bogus percentage.
func TestProgressWriterUnknownTotal(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	var tick int64
	orig := nowFunc
	nowFunc = func() time.Time { return base.Add(time.Duration(tick) * time.Second) }
	defer func() { nowFunc = orig }()

	pw := &progressWriter{w: &bytes.Buffer{}, total: -1, start: nowFunc()}
	tick = 2
	if _, err := pw.Write(make([]byte, 1<<20)); err != nil {
		t.Fatal(err)
	}
	pw.finish() // must not panic
}
