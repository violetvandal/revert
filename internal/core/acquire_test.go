package core

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGameZip builds a .zip whose game data lives under an optional wrapping folder, so
// the nesting path (a zip of "MyTHUG2/Data/pre/...") is exercised, not just a flat one.
func writeGameZip(t *testing.T, path, prefix string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, name := range []string{"Data/pre/qb.prx", "Data/pre/skate.prx", "THUG2.exe"} {
		full := name
		if prefix != "" {
			full = prefix + "/" + name
		}
		w, err := zw.Create(full)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("x"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestIngestSourceFolder(t *testing.T) {
	dir := t.TempDir()
	// a game folder wrapped one level deep, to exercise locateGameRoot
	game := filepath.Join(dir, "src", "MyTHUG2")
	mustWrite(t, filepath.Join(game, "Data", "pre", "qb.prx"), "x")
	dest := filepath.Join(dir, "pristine")

	if err := ingestSource(filepath.Join(dir, "src"), dest); err != nil {
		t.Fatalf("ingestSource(folder): %v", err)
	}
	if !dirExists(filepath.Join(dest, "Data", "pre")) {
		t.Error("Data/pre not present at the pristine root after a nested folder ingest")
	}
}

func TestIngestSourceZipFlatAndNested(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
	}{
		{"flat", ""},
		{"nested", "MyTHUG2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			z := filepath.Join(dir, "game.zip")
			writeGameZip(t, z, tc.prefix)
			dest := filepath.Join(dir, "pristine")

			if err := ingestSource(z, dest); err != nil {
				t.Fatalf("ingestSource(zip): %v", err)
			}
			// The key property: Data/pre lands at the pristine ROOT even when the zip wrapped
			// the game in a top folder.
			if !dirExists(filepath.Join(dest, "Data", "pre")) {
				t.Errorf("%s zip: Data/pre not at pristine root", tc.name)
			}
			if !fileExists(filepath.Join(dest, "Data", "pre", "qb.prx")) {
				t.Errorf("%s zip: expected file missing", tc.name)
			}
		})
	}
}

func TestIngestSourceRejectsNonZipFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "game.7z")
	os.WriteFile(f, []byte("not a zip"), 0o644)
	err := ingestSource(f, filepath.Join(dir, "pristine"))
	if err == nil || !strings.Contains(err.Error(), ".zip") {
		t.Fatalf("err = %v, want guidance mentioning .zip", err)
	}
}

// Rejection is by CONTENT, not URL suffix: a link that serves non-zip bytes (a .7z, an
// HTML error page returned with 200) is refused after the fetch.
func TestDownloadSourceRejectsNonZipContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("7z\xBC\xAF\x27\x1C not really but definitely not a zip"))
	}))
	defer srv.Close()

	c := testConf(t)
	// URL has NO .zip suffix — proving we don't gate on the extension either way.
	if _, err := downloadSource(c, srv.URL+"/download"); err == nil || !strings.Contains(err.Error(), "valid .zip") {
		t.Errorf("downloadSource(non-zip content) err = %v, want a 'valid .zip' rejection", err)
	}
}

// The tinyurl case: an extensionless URL that 302-redirects to a real .zip must work,
// because Go's http client follows the redirect and we identify by content.
func TestDownloadSourceFollowsRedirectToZip(t *testing.T) {
	dir := t.TempDir()
	z := filepath.Join(dir, "game.zip")
	writeGameZip(t, z, "MyTHUG2")
	body, _ := os.ReadFile(z)

	mux := http.NewServeMux()
	mux.HandleFunc("/real.zip", func(w http.ResponseWriter, r *http.Request) { w.Write(body) })
	mux.HandleFunc("/x7Gh2", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/real.zip", http.StatusFound) // stand-in for a link shortener
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := testConf(t)
	dl, err := downloadSource(c, srv.URL+"/x7Gh2") // no .zip suffix, just like tinyurl.com/x7Gh2
	if err != nil {
		t.Fatalf("downloadSource(redirect): %v", err)
	}
	defer os.Remove(dl)

	dest := filepath.Join(c.Root, "pristine-dl")
	if err := ingestSource(dl, dest); err != nil {
		t.Fatalf("ingestSource(downloaded): %v", err)
	}
	if !dirExists(filepath.Join(dest, "Data", "pre")) {
		t.Error("redirected zip did not yield Data/pre at the pristine root")
	}
}
