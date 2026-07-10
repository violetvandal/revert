package core

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// The Windows lane updates by replacing its own binaries from a published GitHub release
// asset — it is an unzipped bundle of prebuilt exes, with no git checkout, no submodules
// and no Go toolchain, so the Linux `git fetch → checkout tag → rebuild` updater
// (share/setup/revert-update.sh) cannot apply. Linux keeps that script as the authority;
// this file is the Windows equivalent.
const (
	defaultUpdateRepo  = "violetvandal/revert"
	windowsBundleAsset = "revert-windows-amd64.zip"
	backupSuffix       = ".old"
	confBackup         = "revert.conf.bak"
)

// UpdateOptions mirrors the bash updater's flags.
type UpdateOptions struct {
	Check bool // report whether a newer release exists, then exit
	Force bool // proceed even from an unstamped dev build
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	HTMLURL string    `json:"html_url"`
	Assets  []ghAsset `json:"assets"`
}

func (r *ghRelease) asset(name string) (ghAsset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return ghAsset{}, false
}

var httpClient = &http.Client{Timeout: 30 * time.Minute}

// githubAPIBase is a var, not a const, so tests can point it at an httptest server.
var githubAPIBase = "https://api.github.com"

func ulog(format string, a ...any)  { fmt.Printf("[update] "+format+"\n", a...) }
func uwarn(format string, a ...any) { fmt.Fprintf(os.Stderr, "[update:warn] "+format+"\n", a...) }

// Update brings the toolkit to the latest published release. On Linux/Steam Deck it hands
// straight to the proven bash updater; on Windows it downloads the release bundle and
// swaps the binaries in place. Game data (the pristine base, the built edition, and your
// Save/) is never read or written.
func Update(c *Conf, o UpdateOptions) error {
	if !IsWindows() {
		var args []string
		if o.Check {
			args = append(args, "--check")
		}
		if o.Force {
			args = append(args, "--force")
		}
		return DelegateToBash(c.Root, "update", args...)
	}
	return updateWindows(c, o)
}

func updateWindows(c *Conf, o UpdateOptions) error {
	// Backups left behind by a previous update (a running .exe can be renamed but not
	// deleted, so its .old lingers until the next run).
	sweepBackups(c.Root)

	repo := c.GetOr("UPDATE_REPO", defaultUpdateRepo)
	ulog("checking %s for the latest release ...", repo)
	rel, err := fetchLatestRelease(repo)
	if err != nil {
		return err
	}

	// --check changes nothing, so it always reports rather than refusing. An unstamped build
	// simply can't say whether the release is newer.
	if IsDevBuild() && o.Check {
		ulog("latest release: %s", rel.TagName)
		ulog("this build is unstamped (%q), so it cannot tell whether that is newer.", Version)
		ulog("to install it anyway: revert update --force")
		return nil
	}

	switch {
	case IsDevBuild() && !o.Force:
		ulog("latest release: %s", rel.TagName)
		return fmt.Errorf("this is an unstamped dev build (version %q), so it cannot tell whether %s is newer — "+
			"updating would risk downgrading your checkout. Re-run with --force to install %s anyway",
			Version, rel.TagName, rel.TagName)
	case IsDevBuild():
		uwarn("dev build — installing %s because --force was given", rel.TagName)
	case compareVersions(Version, rel.TagName) >= 0:
		ulog("already up to date (%s)", Version)
		return nil
	}

	ulog("current: %s   latest release: %s", Version, rel.TagName)
	if o.Check {
		ulog("update available -> run: revert update")
		return nil
	}

	asset, ok := rel.asset(windowsBundleAsset)
	if !ok {
		return fmt.Errorf("release %s publishes no %s asset, so there is nothing to install on Windows. "+
			"See %s", rel.TagName, windowsBundleAsset, rel.HTMLURL)
	}

	work, err := os.MkdirTemp("", "revert-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	zipPath := filepath.Join(work, windowsBundleAsset)
	ulog("downloading %s (%s)", asset.Name, humanBytes(asset.Size))
	if err := download(asset.URL, zipPath); err != nil {
		return fmt.Errorf("downloading %s: %w", asset.Name, err)
	}

	ulog("verifying ...")
	if err := verifyBundle(rel, zipPath); err != nil {
		return err
	}

	stage := filepath.Join(work, "stage")
	if err := extractZip(zipPath, stage); err != nil {
		return fmt.Errorf("extracting %s: %w", asset.Name, err)
	}
	if !fileExists(filepath.Join(stage, "revert.exe")) {
		return fmt.Errorf("%s does not contain revert.exe — refusing to install a bundle that isn't the toolkit", asset.Name)
	}

	// Keep the outgoing revert.conf: it is toolkit-owned and gets replaced, but a user who
	// edited it directly (rather than revert.conf.local) would otherwise lose that silently.
	confReplaced := false
	if src := filepath.Join(c.Root, "revert.conf"); fileExists(src) && fileExists(filepath.Join(stage, "revert.conf")) {
		if err := copyFile(src, filepath.Join(c.Root, confBackup)); err != nil {
			uwarn("could not back up revert.conf: %v", err)
		} else {
			confReplaced = true
		}
	}

	ulog("installing %s ...", rel.TagName)
	if err := installStaged(c, stage); err != nil {
		return err
	}
	sweepBackups(c.Root) // most .old files delete cleanly; running exes wait for next run

	if confReplaced {
		ulog("revert.conf was replaced; your previous copy is %s.", confBackup)
		ulog("machine-specific settings belong in revert.conf.local, which updates never touch.")
	}

	// Rebuild with the NEW binaries (thugkit.exe just changed under us), mirroring the
	// bash updater's final `"${REVERT_ROOT}/revert" build`.
	if dirExists(filepath.Join(c.Path("PRISTINE_DIR"), "Data", "pre")) {
		ulog("rebuilding the edition ...")
		if err := runInherit(c.Root, nil, filepath.Join(c.Root, "revert.exe"), "build"); err != nil {
			return fmt.Errorf("updated to %s, but the rebuild failed: %w (re-run: revert build)", rel.TagName, err)
		}
	} else {
		ulog("no game data yet — run: revert acquire-game-data, then: revert build")
	}

	ulog("updated to %s. Done.", rel.TagName)
	ulog("restart the GUI (revert-gui.exe) if it is open, so it picks up the new version.")
	return nil
}

// ── release metadata ────────────────────────────────────────────────────────────

func fetchLatestRelease(repo string) (*ghRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "revert-updater")

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reaching github.com: %w (check your network)", err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("%s has no published releases yet", repo)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("github rate-limited this machine (HTTP %d) — try again in a few minutes", res.StatusCode)
	default:
		return nil, fmt.Errorf("github returned HTTP %d for %s", res.StatusCode, url)
	}

	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parsing the release feed: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("the latest release of %s has no tag name", repo)
	}
	return &rel, nil
}

func download(url, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "revert-updater")
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	// Report progress as it streams. Game-data downloads are large, and without this the
	// GUI console (and the terminal) sit silent for minutes; a stalled link is
	// indistinguishable from a slow one. res.ContentLength is -1 when the server sends no
	// Content-Length, in which case we print bytes + rate without a percentage.
	pw := &progressWriter{w: f, total: res.ContentLength, start: nowFunc()}
	if _, err := io.Copy(pw, res.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	pw.finish()
	return nil
}

// progressWriter wraps the destination file and emits a one-line progress report at most
// once a second (newline-terminated, not a carriage-return meter — the GUI console renders
// each line, and a \r meter garbles it).
type progressWriter struct {
	w        io.Writer
	total    int64 // -1 if unknown
	n        int64
	start    time.Time
	last     time.Time
	reported bool
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.n += int64(n)
	now := nowFunc()
	if p.last.IsZero() {
		p.last = p.start
	}
	if now.Sub(p.last) >= time.Second {
		p.line(now)
		p.last, p.reported = now, true
	}
	return n, err
}

func (p *progressWriter) line(now time.Time) {
	secs := now.Sub(p.start).Seconds()
	if secs < 0.001 {
		secs = 0.001
	}
	mbps := float64(p.n) / 1e6 / secs
	if p.total > 0 {
		fmt.Printf("  %s / %s  (%d%%)  %.1f MB/s\n",
			humanBytes(p.n), humanBytes(p.total), p.n*100/p.total, mbps)
	} else {
		fmt.Printf("  %s downloaded  %.1f MB/s\n", humanBytes(p.n), mbps)
	}
}

// finish prints a closing summary, but only if at least one progress line was shown (a tiny
// download that finishes in under a second needs no report).
func (p *progressWriter) finish() {
	if !p.reported {
		return
	}
	secs := nowFunc().Sub(p.start).Seconds()
	fmt.Printf("  done: %s in %.0fs\n", humanBytes(p.n), secs)
}

// nowFunc is time.Now, indirected so tests can pin the clock for deterministic rates.
var nowFunc = time.Now

// verifyBundle checks the download against the release's "<asset>.sha256" sidecar when one
// is published, and always checks that the file is a readable zip. A release without a
// sidecar still installs — the sidecar is defence against a corrupted transfer, not
// against a hostile GitHub, which HTTPS already covers.
func verifyBundle(rel *ghRelease, zipPath string) error {
	if sc, ok := rel.asset(windowsBundleAsset + ".sha256"); ok {
		want, err := fetchSHA256(sc.URL)
		if err != nil {
			return fmt.Errorf("fetching the checksum: %w", err)
		}
		got, err := sha256File(zipPath)
		if err != nil {
			return err
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("checksum mismatch: expected %s, got %s — the download is corrupt; try again", want, got)
		}
		ulog("sha256 ok (%s)", got[:16])
	} else {
		uwarn("release publishes no %s.sha256 — skipping the checksum", windowsBundleAsset)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the download is not a readable zip: %w", err)
	}
	return zr.Close()
}

func fetchSHA256(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "revert-updater")
	res, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		return "", err
	}
	// sha256sum format: "<hex>  <filename>"
	if f := strings.Fields(string(b)); len(f) > 0 {
		return f[0], nil
	}
	return "", fmt.Errorf("empty checksum file")
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ── extraction ──────────────────────────────────────────────────────────────────

// extractZip unpacks src into dst, rejecting any entry that would escape dst ("zip slip":
// an archive entry named ../../Windows/System32/...). The staging dir is a fresh temp
// directory, so a rejected archive never reaches the install root.
func extractZip(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		// Normalize separators first: a "..\evil" entry is a traversal on Windows, and
		// backslash is a legal filename character on Linux, so it must be folded to "/"
		// before the check rather than after.
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if name == "" || path.Clean(name) == "." {
			continue
		}
		// filepath.Join cleans, so a "../" entry resolves outside dst and withinDir catches
		// it. Deliberately NOT rooting the name at "/" first: that would silently *rewrite*
		// "../../evil" to "evil" and install it, rather than refusing an archive that has
		// no business containing such an entry.
		target := filepath.Join(dst, filepath.FromSlash(name))
		if !withinDir(dst, target) {
			return fmt.Errorf("archive entry %q escapes the staging directory", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// withinDir reports whether target lies inside dir (or is dir itself).
func withinDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ── install / rollback ──────────────────────────────────────────────────────────

type swap struct{ target, backup string } // backup == "" when target was newly created

// installStaged copies every staged file over the install root, backing up whatever it
// replaces so a mid-way failure rolls back cleanly. Windows cannot delete or overwrite a
// running .exe but it *can* rename one, so each existing file is renamed aside first —
// that is what lets revert.exe replace itself while it is the running process.
func installStaged(c *Conf, stage string) error {
	protected := protectedDirs(c)
	var done []swap

	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			s := done[i]
			os.Remove(s.target)
			if s.backup != "" {
				if err := os.Rename(s.backup, s.target); err != nil {
					uwarn("rollback: could not restore %s from %s: %v", s.target, s.backup, err)
				}
			}
		}
	}

	err := filepath.Walk(stage, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil {
			return err
		}
		target := filepath.Join(c.Root, rel)

		// Belt and braces. The bundle contains only tooling, so these paths can't appear in
		// it — but an updater that can write into game-playable-us/Save/ is one bad archive
		// away from eating the user's save file.
		for _, guard := range protected {
			if guard != "" && withinDir(guard, target) {
				return fmt.Errorf("refusing to write %s: it is inside the protected directory %s", target, guard)
			}
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		s := swap{target: target}
		if fileExists(target) {
			s.backup = target + backupSuffix
			os.Remove(s.backup) // a stale backup would block the rename
			if err := os.Rename(target, s.backup); err != nil {
				return fmt.Errorf("replacing %s: %w", target, err)
			}
		}
		if err := copyFile(p, target); err != nil {
			// Put the original back before unwinding the rest.
			if s.backup != "" {
				os.Rename(s.backup, target)
			}
			return fmt.Errorf("writing %s: %w", target, err)
		}
		done = append(done, s)
		return nil
	})

	if err != nil {
		uwarn("install failed, rolling back ...")
		rollback()
		return err
	}
	return nil
}

// protectedDirs lists the paths an update must never write into: the game base, both built
// editions (which hold Save/), and the online lane.
func protectedDirs(c *Conf) []string {
	var out []string
	for _, k := range []string{"PRISTINE_DIR", "EDITION_QOL", "EDITION_VANILLA", "LANE_ONLINE_DIR"} {
		if v := c.Path(k); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// sweepBackups deletes leftover *.old files. Ones belonging to a still-running executable
// stay locked and are skipped silently; the next update sweeps them.
func sweepBackups(root string) {
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a sweep must never fail the update
		}
		if fi.IsDir() {
			// Don't descend into the game trees; they're big and hold nothing of ours.
			if name := fi.Name(); strings.HasPrefix(name, "game-") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, backupSuffix) {
			os.Remove(p)
		}
		return nil
	})
}

func humanBytes(n int64) string {
	switch {
	case n <= 0:
		return "unknown size"
	case n < 1<<20:
		return fmt.Sprintf("%d KB", n>>10)
	default:
		return fmt.Sprintf("%d MB", n>>20)
	}
}
