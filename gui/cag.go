package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The Create-A-Graphic studio: import an image as a spray tag or a wall-slap
// sticker, see what it will actually look like in game, and give a slot back.
//
// Everything routes through the `revert` dispatcher rather than reaching into
// the game, so the browser and the terminal drive exactly the same code and
// cannot disagree about which slot an import holds.

// maxUpload caps an uploaded source image. Imports are downscaled to a 64px
// sprite, so nothing near this is ever needed; the limit is here so a stray
// large file cannot exhaust memory.
const maxUpload = 32 << 20 // 32 MiB

// safeName keeps uploaded filenames to something that cannot escape the temp
// dir or confuse the CLI.
var safeName = regexp.MustCompile(`[^A-Za-z0-9 ._-]`)

func sanitizeUpload(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = safeName.ReplaceAllString(name, "")
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "image.png"
	}
	return name
}

// stashUpload writes a posted image to a temp file and returns its path.
func stashUpload(r *http.Request) (path string, cleanup func(), err error) {
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		return "", func() {}, err
	}
	f, hdr, err := r.FormFile("image")
	if err != nil {
		return "", func() {}, err
	}
	defer f.Close()

	dir, err := os.MkdirTemp("", "revert-cag")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { os.RemoveAll(dir) }

	path = filepath.Join(dir, sanitizeUpload(hdr.Filename))
	out, err := os.Create(path)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(f, maxUpload)); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

// revertCAG runs a `revert` subcommand and returns its combined output.
func revertCAG(args ...string) ([]byte, error) {
	root := effectiveRoot()
	if root == "" {
		return nil, fmt.Errorf("no Revert install found yet")
	}
	c := newCmd("Revert clip-art", filepath.Join(root, revertBinName()), args...)
	c.Dir = root
	c.Env = guiEnv()
	return c.CombinedOutput()
}

func httpErr(w http.ResponseWriter, code int, err error, out []byte) {
	msg := err.Error()
	if len(out) > 0 {
		msg = strings.TrimSpace(string(out))
	}
	msg = friendly(msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// cagListHandler: GET /api/cag/list -> the state of every slot in the pool.
func cagListHandler(w http.ResponseWriter, r *http.Request) {
	out, err := revertCAG("cag", "list", "--json")
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err, out)
		return
	}
	// The dispatcher may print its own log lines first; the JSON starts at the
	// first bracket.
	if i := strings.IndexAny(string(out), "[{"); i > 0 {
		out = out[i:]
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// cagPreviewHandler: POST /api/cag/preview -> a PNG of what the game will show.
//
// The preview is the encoded sprite decoded back, not the uploaded file, so the
// palette reduction and the downscale are visible before anything is imported.
func cagPreviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	src, cleanup, err := stashUpload(r)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err, nil)
		return
	}
	defer cleanup()

	out := filepath.Join(filepath.Dir(src), "preview.png")
	if o, err := revertCAG("cag", "preview", src, "--out", out); err != nil {
		httpErr(w, http.StatusInternalServerError, err, o)
		return
	}
	png, err := os.ReadFile(out)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err, nil)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

// cagSpriteHandler: GET /api/cag/sprite?slot=grap_1 -> that slot's current art.
func cagSpriteHandler(w http.ResponseWriter, r *http.Request) {
	slot := r.URL.Query().Get("slot")
	if !regexp.MustCompile(`^grap_[0-9]{1,3}$`).MatchString(slot) {
		http.Error(w, "bad slot", http.StatusBadRequest)
		return
	}
	dir, err := os.MkdirTemp("", "revert-sprite")
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err, nil)
		return
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, slot+".png")
	if o, err := revertCAG("cag", "sprite", slot, "--out", out); err != nil {
		httpErr(w, http.StatusInternalServerError, err, o)
		return
	}
	png, err := os.ReadFile(out)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err, nil)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

// cagImportHandler: POST /api/cag/import -> import an image and report its slot.
func cagImportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	src, cleanup, err := stashUpload(r)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err, nil)
		return
	}
	defer cleanup()

	cmd := "tag"
	if r.FormValue("kind") == "sticker" {
		cmd = "sticker"
	}
	args := []string{cmd, src, "--install"}
	if n := strings.TrimSpace(r.FormValue("name")); n != "" {
		args = append(args, "--name", sanitizeUpload(n))
	}
	out, err := revertCAG(args...)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err, out)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"log": strings.TrimSpace(string(out))})
}

// cagRemoveHandler: POST /api/cag/remove -> drop an import, restoring stock art.
func cagRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	out, err := revertCAG("cag", "remove", sanitizeUpload(name))
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err, out)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"log": strings.TrimSpace(string(out))})
}

// friendly rewrites the handful of tool errors a person can actually cause
// into something that says what to do about it. Anything unrecognised is
// passed through unchanged rather than swallowed.
func friendly(msg string) string {
	switch {
	case strings.Contains(msg, "image: unknown format"),
		strings.Contains(msg, "unexpected EOF"):
		return "That file is not an image the toolkit can read. PNG, JPEG, GIF and BMP all work."
	case strings.Contains(msg, "no Revert install found"):
		return "No Revert installation found yet. Finish the install steps first."
	case strings.Contains(msg, "cagpieces.prx"):
		return "Your game folder is missing its clip-art archive. Run a build first."
	case strings.Contains(msg, "pool is full"), strings.Contains(msg, "exceeds the"):
		return "Every clip-art slot is taken. Remove an import before adding another."
	case strings.Contains(msg, "no import named"):
		return "There is no import by that name any more; the list may be out of date."
	}
	return msg
}
