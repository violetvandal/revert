package core

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AcquireOptions mirror `revert acquire-game-data`.
type AcquireOptions struct {
	From string // a folder holding your THUG2 install, or a .zip of it
	URL  string // a link to a .zip of your THUG2 install, to download then ingest
}

// Acquire turns the user's own THUG2 copy into the pristine base. On Linux it delegates
// to share/setup/revert-acquire.sh (the ISO/MSI/rsync/url recipe). On Windows it runs
// natively: a folder or .zip via --from, or a .zip fetched from --url (the "download my
// own copy" path). ISO/7z stay "extract yourself, then --from".
func Acquire(c *Conf, o AcquireOptions) error {
	if IsLinux() {
		args := []string{}
		if o.URL != "" {
			args = append(args, "--url", o.URL)
		} else if o.From != "" {
			args = append(args, "--folder", o.From)
		}
		return DelegateToBash(c.Root, "acquire-game-data", args...)
	}

	dest := c.Path("PRISTINE_DIR")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	src := o.From
	if o.URL != "" {
		dl, err := downloadSource(c, o.URL)
		if err != nil {
			return err
		}
		defer os.Remove(dl)
		src = dl
	}
	if src == "" {
		return fmt.Errorf("usage: revert acquire-game-data --from <your THUG2 folder or .zip> | --url <link to a .zip>")
	}

	if err := ingestSource(src, dest); err != nil {
		return err
	}
	if !dirExists(filepath.Join(dest, "Data", "pre")) {
		return fmt.Errorf("acquired data has no Data/pre under %s — is this a THUG2 install folder (or a .zip of one)?", dest)
	}
	fmt.Printf("[revert] pristine base ready: %s\n", dest)
	return nil
}

// downloadSource fetches a URL to a scratch file next to the pristine dir (real disk, not
// %TEMP% which is often small), and returns its path for the caller to ingest + delete.
//
// The archive is identified by CONTENT, not by the URL's extension: a link shortener
// (tinyurl, a GitHub release "latest" link, a Drive share) redirects to a .zip while the
// URL itself has no .zip suffix. Go's http client follows the redirects; we then confirm
// the downloaded file is actually a valid zip (Windows' native ingest only does zip).
func downloadSource(c *Conf, url string) (string, error) {
	dst := filepath.Join(filepath.Dir(c.Path("PRISTINE_DIR")), ".revert-acquire-download.zip")
	os.Remove(dst)
	fmt.Printf("[revert] downloading %s\n", url)
	if err := download(url, dst); err != nil {
		os.Remove(dst)
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	// zip.OpenReader validates the end-of-central-directory, so this rejects a .7z/.iso, an
	// HTML error page served with a 200, or a truncated download — not just a wrong suffix.
	r, err := zip.OpenReader(dst)
	if err != nil {
		os.Remove(dst)
		return "", fmt.Errorf("the link did not download a valid .zip (%v). Windows supports only .zip downloads; "+
			"for a .7z/.iso, download and extract it yourself, then use --from <folder>: %s", err, url)
	}
	r.Close()
	return dst, nil
}

// ingestSource copies a THUG2 install (a folder, or a .zip of one) into dest. A .zip is
// unpacked to a scratch dir first, so locateGameRoot can descend into a wrapping top
// folder the same way it does for a folder source — otherwise a zip whose game lives under
// a subfolder would land Data/pre one level too deep and fail the caller's sanity check.
func ingestSource(src, dest string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("cannot read source %q: %w", src, err)
	}
	if fi.IsDir() {
		root := locateGameRoot(src)
		fmt.Printf("[revert] copying %s -> %s\n", root, dest)
		return copyTree(root, dest)
	}
	if strings.HasSuffix(strings.ToLower(src), ".zip") {
		tmp, err := os.MkdirTemp(filepath.Dir(dest), ".revert-unzip-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		fmt.Printf("[revert] extracting %s\n", filepath.Base(src))
		if err := unzipInto(src, tmp); err != nil {
			return err
		}
		root := locateGameRoot(tmp)
		fmt.Printf("[revert] copying game data -> %s\n", dest)
		return copyTree(root, dest)
	}
	return fmt.Errorf("unsupported source %q — pass a folder or a .zip (for .iso/.7z, extract it yourself then use --from <folder>)", src)
}

// locateGameRoot finds the directory that actually holds Data/pre, descending one level
// if the source wraps the game in a single subfolder.
func locateGameRoot(dir string) string {
	if dirExists(filepath.Join(dir, "Data", "pre")) {
		return dir
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() && dirExists(filepath.Join(dir, e.Name(), "Data", "pre")) {
			return filepath.Join(dir, e.Name())
		}
	}
	return dir
}

// unzipInto extracts a .zip into dest, guarding against Zip-Slip path traversal.
func unzipInto(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	destAbs, _ := filepath.Abs(dest)
	for _, f := range r.File {
		target := filepath.Join(dest, filepath.FromSlash(f.Name))
		absTarget, _ := filepath.Abs(target)
		if !strings.HasPrefix(absTarget, destAbs+string(os.PathSeparator)) && absTarget != destAbs {
			return fmt.Errorf("unsafe path in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		if err := extractZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
