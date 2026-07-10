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
	"sync/atomic"
	"time"
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

// revertBinName is the dispatcher binary's name: the native Go `revert.exe` on Windows
// (there's no bash there to run the shebang script), the bash `revert` elsewhere.
func revertBinName() string {
	if runtime.GOOS == "windows" {
		return "revert.exe"
	}
	return "revert"
}

// revertAt reports whether dir holds an executable `revert` dispatcher.
func revertAt(dir string) bool {
	if dir == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(dir, revertBinName()))
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

// newCmd builds a command, wrapped in `systemd-inhibit` when that's available, so a long
// install or build isn't cut off by the Deck idle-suspending (logind sleep/idle inhibit,
// which KDE's power management honors). Falls back to a plain command off systemd.
func newCmd(why, name string, args ...string) *exec.Cmd {
	if p, err := exec.LookPath("systemd-inhibit"); err == nil {
		full := append([]string{"--what=idle:sleep", "--why=" + why, "--mode=block", name}, args...)
		return exec.Command(p, full...)
	}
	return exec.Command(name, args...)
}

// guiEnv augments the process env with the local Go + bin dirs a fresh Linux install
// drops, so `revert build` (which needs the Go toolchain) works from the GUI
// post-install. On Windows the bundle ships prebuilt binaries and needs no Go toolchain,
// so the process env is used as-is (and the ':' PATH separator would corrupt a Windows
// PATH anyway).
func guiEnv() []string {
	env := os.Environ()
	if runtime.GOOS == "windows" {
		return env
	}
	if home, err := os.UserHomeDir(); err == nil {
		sep := string(os.PathListSeparator)
		extra := filepath.Join(home, ".local", "go", "bin") + sep + filepath.Join(home, ".local", "bin")
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + strings.TrimPrefix(e, "PATH=") + sep + extra
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
	mux.HandleFunc("/api/heartbeat", heartbeatHandler)

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
	fmt.Printf("Revert GUI running at %s\n  mode: %s\n", url, mode)
	// On Windows the GUI is launched by double-clicking revert-gui.exe, which leaves a
	// console window sitting behind the browser. Exit when the browser tab closes so that
	// window closes too. On Linux/Deck the GUI is launched from a terminal or a .desktop
	// whose lifecycle the user manages, so keep the Ctrl-C behavior there, untouched.
	if runtime.GOOS == "windows" {
		fmt.Println("(closes automatically when you close the browser tab)")
		go watchBrowser()
	} else {
		fmt.Println("(Ctrl-C to quit)")
	}
	openBrowser(url)
	if err := http.Serve(ln, mux); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
	}
}

// Browser-presence tracking. The page heartbeats while it is open; when the beats stop
// (tab closed) and nothing is mid-run, the process exits so its console window closes.
var (
	lastBeat   atomic.Int64 // UnixNano of the last heartbeat; 0 means "browser said goodbye"
	sawBrowser atomic.Bool  // set once the page has connected at least once
	activeCmds atomic.Int32 // >0 while a command is streaming (a build/install in flight)
)

func heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	sawBrowser.Store(true)
	if r.URL.Query().Get("bye") == "1" {
		lastBeat.Store(0) // pagehide beacon: the tab is closing, exit as soon as we're idle
	} else {
		lastBeat.Store(time.Now().UnixNano())
	}
	w.WriteHeader(http.StatusNoContent)
}

// watchBrowser exits the process a few seconds after the browser tab goes away. It never
// interrupts work: a running command (build/install) holds activeCmds > 0, and the child
// process keeps streaming to completion even with no browser attached (streamCmd reads its
// stdout regardless), so we wait until it finishes before exiting. The grace window rides
// out a page reload, which resumes heartbeats within ~2s.
func watchBrowser() {
	const grace = 6 * time.Second
	for range time.Tick(2 * time.Second) {
		if !sawBrowser.Load() || activeCmds.Load() > 0 {
			continue
		}
		beat := lastBeat.Load()
		if beat == 0 || time.Since(time.Unix(0, beat)) > grace {
			fmt.Println("Browser closed — shutting down.")
			os.Exit(0)
		}
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
		"os":          runtime.GOOS, // "windows" -> the UI hides the sudo-password field
		"version":     revertVersion(root),
	})
}

// revertVersion asks the installed CLI which release it was built from ("dev" for an
// unstamped build). Empty when the toolkit isn't installed yet or the binary is too old to
// know its own version.
func revertVersion(root string) string {
	if root == "" {
		return ""
	}
	out, err := exec.Command(filepath.Join(root, revertBinName()), "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	c := exec.Command(filepath.Join(root, revertBinName()), "status", "--json")
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
	if runtime.GOOS == "windows" {
		// The standalone bootstrap (bash install.sh + sudo/askpass) is Linux/Deck-only.
		// On Windows the bundle already carries revert.exe, so the GUI runs as the
		// management panel and drives setup/acquire/build directly — no wizard needed.
		sse(w, flusher, "error", "On Windows, extract the bundle and use the management panel (Setup → Acquire → Build → Play). The bootstrap wizard is Linux/Steam Deck only.")
		sse(w, flusher, "done", "1")
		return
	}
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
	c := newCmd("THUG2: Violet Vandal Edition is installing", "bash", installSh)
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
	activeCmds.Add(1) // hold off the browser-close watchdog while this command runs
	defer activeCmds.Add(-1)
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

	// Wrap in systemd-inhibit too: `build` is long and `run` should hold off suspend
	// while the game is up.
	c := newCmd("Revert: "+cmd, filepath.Join(root, revertBinName()), args...)
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

// ── native folder picker ──────────────────────────────────────────────────────

// winFolderPickerPS drives a Windows folder dialog from this helper process. The catch:
// the GUI runs in the *browser*, so revert-gui.exe is never the foreground process, and
// Windows blocks a background process from pulling its dialog to the front. The dialog
// then opens behind the browser and the button hangs waiting on a dialog nobody can see.
//
// Two things beat that. A real, opaque, off-screen TopMost owner form (the old code used
// an Opacity=0 owner — an invisible layered window that did not reliably pass its z-order
// to the child dialog). And the AttachThreadInput trick: temporarily attach our input
// queue to the current foreground thread so SetForegroundWindow is allowed to move focus
// to our owner, which the modal dialog then inherits.
const winFolderPickerPS = `
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -Namespace VV -Name Native -MemberDefinition @'
[DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
[DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, IntPtr pid);
[DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
[DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a, uint b, bool attach);
[DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
'@
$owner = New-Object System.Windows.Forms.Form
$owner.TopMost = $true
$owner.ShowInTaskbar = $false
$owner.FormBorderStyle = 'None'
$owner.StartPosition = 'Manual'
$owner.Left = -3000; $owner.Top = -3000; $owner.Width = 1; $owner.Height = 1
$owner.Show()
$fg = [VV.Native]::GetForegroundWindow()
$ft = [VV.Native]::GetWindowThreadProcessId($fg, [IntPtr]::Zero)
$mt = [VV.Native]::GetCurrentThreadId()
[void][VV.Native]::AttachThreadInput($mt, $ft, $true)
[void][VV.Native]::SetForegroundWindow($owner.Handle)
$owner.Activate()
[void][VV.Native]::AttachThreadInput($mt, $ft, $false)
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Select your THUG2 folder'
$d.ShowNewFolderButton = $false
$r = $d.ShowDialog($owner)
$owner.Close()
if ($r -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.SelectedPath) }
`

// firstLine returns the first non-empty line of s (PowerShell error text is multi-line;
// the first line is the useful one for a toast).
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return strings.TrimSpace(s)
}

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
		c = exec.Command("powershell", "-NoProfile", "-STA", "-Command", winFolderPickerPS)
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
		// A cancelled dialog exits 0 with empty output, so a non-zero exit is a real
		// failure (a picker exception, a missing tool). Surface its stderr instead of
		// swallowing it as "no selection" — that silence is exactly what made a broken
		// picker look like a dead button.
		if ee, ok := err.(*exec.ExitError); ok {
			if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
				return "", fmt.Errorf("folder picker: %s", firstLine(msg))
			}
		}
		return "", nil
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

// openBrowser opens url in the default browser. Implemented per-platform: on Windows via
// the ShellExecute API directly (browser_windows.go) — NOT `rundll32 url.dll,...`, which
// Windows Defender's behavior monitor flags as a defense-evasion / proxy-execution
// technique and quarantines the (unsigned) exe. See browser_other.go for macOS/Linux.
