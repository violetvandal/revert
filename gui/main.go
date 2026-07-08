// revert-gui — a tiny, zero-dependency front-end for the Revert toolkit.
//
// Two faces, one binary:
//   - Installed (launched via `revert gui` from a clone): a management panel that
//     drives the `revert` CLI (doctor/setup/acquire/build/run/update), streaming
//     output live to the browser via SSE.
//   - Standalone (downloaded on its own, no clone yet): a first-run install wizard.
//     It collects a game source + the account password once, then runs the bootstrap
//     (install.sh) in non-interactive "driven" mode — cloning the repo, installing Go,
//     and running setup/build — feeding sudo through SUDO_ASKPASS so there's no terminal.
//     This is the "just download and click" path, Steam-Deck friendly.
//
// Pure Go stdlib — no CGO, no native deps. `go build` -> one static binary that
// cross-compiles to Linux / Steam Deck / Windows / macOS.
package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

//go:embed web
var webFS embed.FS

// subcommands the GUI is allowed to invoke (whitelist — never exec arbitrary input).
var allowed = map[string]bool{
	"doctor": true, "setup": true, "acquire-game-data": true, "acquire-hq": true,
	"build": true, "run": true, "update": true, "tag": true, "help": true,
	"calibrate-controller": true,
}

// ── install-location state ──────────────────────────────────────────────────
// installedDir is set once a bootstrap finishes so the management endpoints target
// the freshly-cloned repo without a restart. Guarded because the SSE install handler
// writes it while other requests read it.
var (
	mu           sync.Mutex
	installedDir string
)

// revertAt reports whether dir holds an executable `revert` dispatcher.
func revertAt(dir string) bool {
	if dir == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(dir, "revert"))
	return err == nil && !fi.IsDir()
}

// effectiveRoot resolves the toolkit root, or "" if the toolkit isn't installed yet.
// Preference: a just-installed dir, then $REVERT_ROOT, then next to the executable
// (dev: running inside the repo), then the default install location (~/thug2).
func effectiveRoot() string {
	mu.Lock()
	d := installedDir
	mu.Unlock()
	if revertAt(d) {
		return d
	}
	if r := os.Getenv("REVERT_ROOT"); revertAt(r) {
		abs, err := filepath.Abs(r)
		if err == nil {
			return abs
		}
		return r
	}
	var cands []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands, d, filepath.Dir(d)) // gui/ and repo root
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands, wd, filepath.Dir(wd))
	}
	for _, c := range cands {
		if revertAt(c) {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	if def := defaultInstallDir(); revertAt(def) {
		return def
	}
	return ""
}

// defaultInstallDir is where a fresh install lands (matches install.sh's $HOME/thug2).
func defaultInstallDir() string {
	if d := os.Getenv("REVERT_DIR"); d != "" {
		return d
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "thug2")
	}
	return "thug2"
}

// guiEnv augments the process env with the local Go + bin dirs a fresh install drops,
// so `revert build` (which needs the Go toolchain) works from the GUI post-install.
func guiEnv() []string {
	env := os.Environ()
	if home, err := os.UserHomeDir(); err == nil {
		extra := filepath.Join(home, ".local", "go", "bin") + ":" + filepath.Join(home, ".local", "bin")
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + strings.TrimPrefix(e, "PATH=") + ":" + extra
				return env
			}
		}
		env = append(env, "PATH="+extra)
	}
	return env
}

func main() {
	mux := http.NewServeMux()

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		fmt.Fprintln(os.Stderr, "embed error:", err)
		os.Exit(1)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/info", infoHandler)
	mux.HandleFunc("/api/status", statusHandler)
	mux.HandleFunc("/api/pwstatus", pwStatusHandler)
	mux.HandleFunc("/api/pick", pickHandler)
	mux.HandleFunc("/api/stream", streamHandler)
	mux.HandleFunc("/api/install/start", installStartHandler)
	mux.HandleFunc("/api/install/stream", installStreamHandler)

	// Bind to loopback on a free port (local, single-user; never exposed).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	url := "http://" + ln.Addr().String()
	mode := "installed"
	if effectiveRoot() == "" {
		mode = "installer (no toolkit found yet)"
	}
	fmt.Printf("Revert GUI running at %s\n  mode: %s\n(Ctrl-C to quit)\n", url, mode)
	openBrowser(url)
	if err := http.Serve(ln, mux); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
	}
}

// infoHandler reports the toolkit root (empty if not installed), whether it was found,
// the default install location, and whether this looks like a Steam Deck.
func infoHandler(w http.ResponseWriter, r *http.Request) {
	root := effectiveRoot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"root":        root,
		"revertFound": root != "",
		"defaultDir":  defaultInstallDir(),
		"isSteamDeck": isSteamDeck(),
		"desktopMode": isDesktopMode(),
	})
}

// isDesktopMode reports a Steam Deck currently in Desktop Mode (KDE) rather than Gaming
// Mode. Gaming Mode runs the gamescope compositor; Desktop Mode does not. Only meaningful
// on a Deck (false elsewhere) — used to tell the user to switch back to Gaming Mode.
func isDesktopMode() bool {
	if !isSteamDeck() {
		return false
	}
	return exec.Command("pgrep", "-x", "gamescope").Run() != nil // no gamescope → Desktop Mode
}

// statusHandler runs `revert status --json` and forwards the lifecycle state the
// front-end uses to gate steps. Returns {} if the toolkit isn't installed yet.
func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	root := effectiveRoot()
	if root == "" {
		fmt.Fprint(w, "{}")
		return
	}
	c := exec.Command(filepath.Join(root, "revert"), "status", "--json")
	c.Dir = root
	c.Env = append(guiEnv(), "REVERT_ROOT="+root, "TERM=dumb")
	out, err := c.Output()
	if err != nil || len(out) == 0 {
		fmt.Fprint(w, "{}")
		return
	}
	w.Write(out)
}

// pwStatusHandler reports whether the current account has NO password yet (fresh Steam
// Deck 'deck' user) so the wizard can label the field "create a password" vs "your
// password". Either way the wizard collects one — setup's sudo needs it.
func pwStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	noPass := false
	if runtime.GOOS != "windows" {
		if out, err := exec.Command("passwd", "-S").Output(); err == nil {
			f := strings.Fields(string(out))
			if len(f) >= 2 && (f[1] == "NP" || f[1] == "L") {
				noPass = true
			}
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"noPassword": noPass})
}

// isSteamDeck mirrors the shell detection (SteamOS env or DMI board name).
func isSteamDeck() bool {
	if os.Getenv("SteamDeck") == "1" {
		return true
	}
	b, _ := os.ReadFile("/sys/devices/virtual/dmi/id/product_name")
	n := strings.ToLower(string(b))
	return strings.Contains(n, "jupiter") || strings.Contains(n, "galileo")
}

// ── install wizard (standalone bootstrap) ────────────────────────────────────

type installReq struct {
	Dir      string `json:"dir"`
	Password string `json:"password"`
	Src      string `json:"src"`
}

// Pending install jobs, keyed by a one-shot token. The password can't ride in the SSE
// URL (EventSource is GET-only and URLs get logged), so the wizard POSTs the request
// here, gets a token, then opens the SSE stream with just the token.
var (
	jobMu  sync.Mutex
	jobs   = map[string]installReq{}
	jobSeq int
)

func installStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req installReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Dir = strings.TrimSpace(req.Dir)
	req.Src = strings.TrimSpace(req.Src)
	if req.Dir == "" {
		req.Dir = defaultInstallDir()
	}
	if req.Password == "" {
		http.Error(w, "a password is required (setup installs system libraries via sudo)", http.StatusBadRequest)
		return
	}
	if req.Src == "" {
		http.Error(w, "point the installer at your THUG2 copy (a folder path or a download link)", http.StatusBadRequest)
		return
	}
	jobMu.Lock()
	jobSeq++
	token := "job" + strconv.Itoa(jobSeq)
	jobs[token] = req
	jobMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func installStreamHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	jobMu.Lock()
	req, ok := jobs[token]
	delete(jobs, token) // one-shot
	jobMu.Unlock()

	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if !ok {
		sse(w, flusher, "error", "install session expired — reload and try again")
		sse(w, flusher, "done", "1")
		return
	}

	// Password → askpass helper. The helper feeds sudo (via SUDO_ASKPASS + `sudo -A`)
	// without a terminal. Kept in a 0600 temp file (not the URL/env) and shredded after.
	askpass, cleanup, err := writeAskpass(req.Password)
	if err != nil {
		sse(w, flusher, "error", "could not prepare sudo helper: "+err.Error())
		sse(w, flusher, "done", "1")
		return
	}
	defer cleanup()

	installSh, tmpScript, err := resolveInstallSh()
	if err != nil {
		sse(w, flusher, "error", err.Error())
		sse(w, flusher, "done", "1")
		return
	}
	if tmpScript != "" {
		defer os.Remove(tmpScript)
	}

	sse(w, flusher, "start", "installing THUG2: Violet Vandal Edition → "+req.Dir)
	c := exec.Command("bash", installSh)
	c.Env = append(guiEnv(),
		"REVERT_DRIVEN=1",
		"REVERT_DIR="+req.Dir,
		"REVERT_PASSWORD="+req.Password, // install.sh uses this for the one-time `passwd`
		"REVERT_GAME_SRC="+req.Src,
		"SUDO_ASKPASS="+askpass,
		"TERM=dumb",
	)
	exit := streamCmd(w, flusher, c)
	if exit == "0" {
		mu.Lock()
		installedDir = req.Dir
		mu.Unlock()
	}
	sse(w, flusher, "done", exit)
}

// writeAskpass writes the password to a 0600 temp file and a 0700 helper script that
// prints it, returning (helperPath, cleanup). SUDO_ASKPASS points at the helper.
func writeAskpass(password string) (string, func(), error) {
	pwFile, err := os.CreateTemp("", "revert-pw-*")
	if err != nil {
		return "", func() {}, err
	}
	pwFile.Chmod(0o600)
	pwFile.WriteString(password + "\n")
	pwFile.Close()

	helper, err := os.CreateTemp("", "revert-askpass-*.sh")
	if err != nil {
		os.Remove(pwFile.Name())
		return "", func() {}, err
	}
	helper.WriteString("#!/bin/sh\ncat " + shellQuote(pwFile.Name()) + "\n")
	helper.Close()
	os.Chmod(helper.Name(), 0o700)

	cleanup := func() {
		// Best-effort shred: overwrite the password file before removing.
		if f, e := os.OpenFile(pwFile.Name(), os.O_WRONLY, 0o600); e == nil {
			io.WriteString(f, strings.Repeat("0", len(password)+1))
			f.Close()
		}
		os.Remove(pwFile.Name())
		os.Remove(helper.Name())
	}
	return helper.Name(), cleanup, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// resolveInstallSh returns a path to a runnable install.sh. In dev (running inside the
// repo) it uses the local one; otherwise it downloads the published installer, matching
// the `bash <(curl … install.sh)` path exactly. Returns (path, tmpPathToRemove, err).
func resolveInstallSh() (string, string, error) {
	// Local sibling: <exe dir>/../install.sh or the repo root next to the running GUI.
	var roots []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		roots = append(roots, filepath.Dir(d), d)
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Dir(wd), wd)
	}
	for _, root := range roots {
		p := filepath.Join(root, "install.sh")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, "", nil
		}
	}
	// Download the published bootstrap.
	url := os.Getenv("REVERT_INSTALL_SH_URL")
	if url == "" {
		url = "https://raw.githubusercontent.com/violetvandal/revert/main/install.sh"
	}
	resp, err := http.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("could not fetch the installer script (%s): %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("could not fetch the installer script: %s returned %s", url, resp.Status)
	}
	tmp, err := os.CreateTemp("", "revert-install-*.sh")
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", "", err
	}
	tmp.Close()
	return tmp.Name(), tmp.Name(), nil
}

// ── management stream (drive the installed `revert` CLI) ──────────────────────

func streamHandler(w http.ResponseWriter, r *http.Request) {
	cmd := r.URL.Query().Get("cmd")
	if !allowed[cmd] {
		http.Error(w, "command not allowed", http.StatusForbidden)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	root := effectiveRoot()
	if root == "" {
		sse(w, flusher, "error", "toolkit not installed yet — use the install wizard first")
		sse(w, flusher, "done", "1")
		return
	}
	args := []string{cmd}
	args = append(args, r.URL.Query()["arg"]...)
	sse(w, flusher, "start", "revert "+strings.Join(args, " "))

	c := exec.Command(filepath.Join(root, "revert"), args...)
	c.Dir = root
	c.Env = append(guiEnv(), "REVERT_ROOT="+root, "TERM=dumb")
	sse(w, flusher, "done", streamCmd(w, flusher, c))
}

// streamCmd runs c, forwarding merged stdout/stderr as SSE "log" events, and returns
// the exit code as a string ("0" on success).
func streamCmd(w http.ResponseWriter, flusher http.Flusher, c *exec.Cmd) string {
	stdout, _ := c.StdoutPipe()
	c.Stderr = c.Stdout
	if err := c.Start(); err != nil {
		sse(w, flusher, "error", err.Error())
		return "1"
	}
	scan := bufio.NewScanner(stdout)
	scan.Buffer(make([]byte, 64*1024), 1024*1024)
	for scan.Scan() {
		sse(w, flusher, "log", stripANSI(scan.Text()))
	}
	if err := c.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return strconv.Itoa(ee.ExitCode())
		}
		return "1"
	}
	return "0"
}

// sse writes one Server-Sent Event, splitting multi-line data into separate data lines.
func sse(w io.Writer, flusher http.Flusher, event, data string) {
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "event: %s\ndata: %s\n", event, line)
	}
	fmt.Fprint(w, "\n")
	flusher.Flush()
}

// ── native folder picker (unchanged) ──────────────────────────────────────────

func pickHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path, err := pickFolder()
	resp := map[string]string{"path": path}
	if err != nil {
		resp["error"] = err.Error()
	}
	b, _ := json.Marshal(resp)
	w.Write(b)
}

func pickFolder() (string, error) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("osascript", "-e",
			`POSIX path of (choose folder with prompt "Select your THUG2 folder")`)
	case "windows":
		ps := `Add-Type -AssemblyName System.Windows.Forms;` +
			`$d = New-Object System.Windows.Forms.FolderBrowserDialog;` +
			`$d.Description = 'Select your THUG2 folder';` +
			`if ($d.ShowDialog() -eq 'OK') { [Console]::Out.Write($d.SelectedPath) }`
		c = exec.Command("powershell", "-NoProfile", "-STA", "-Command", ps)
	default: // linux / *bsd
		if p, _ := exec.LookPath("zenity"); p != "" {
			c = exec.Command(p, "--file-selection", "--directory",
				"--title=Select your THUG2 folder")
		} else if p, _ := exec.LookPath("kdialog"); p != "" {
			c = exec.Command(p, "--getexistingdirectory", os.Getenv("HOME"))
		} else {
			return "", fmt.Errorf("no folder dialog found — install zenity or kdialog, or type the path")
		}
	}
	out, err := c.Output()
	if err != nil {
		return "", nil // treat cancel / non-zero as "no selection", not an error
	}
	return strings.TrimSpace(string(out)), nil
}

// stripANSI removes terminal color escapes so the log reads cleanly in the browser.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b { // ESC
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	c.Stdout, c.Stderr = io.Discard, io.Discard
	_ = c.Start()
}
