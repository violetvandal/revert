// revert-gui — a tiny, zero-dependency desktop front-end for the Revert toolkit.
//
// It serves a local web page and drives the `revert` CLI (doctor/setup/acquire/
// build/run/update), streaming command output live to the browser via SSE. The
// CLI is the seam: this binary adds no logic of its own, just a friendly face.
//
// Pure Go stdlib — no CGO, no native deps. `go build` -> one static binary that
// cross-compiles to Linux / Steam Deck / Windows / macOS.
package main

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed web
var webFS embed.FS

// subcommands the GUI is allowed to invoke (whitelist — never exec arbitrary input).
var allowed = map[string]bool{
	"doctor": true, "setup": true, "acquire-game-data": true, "acquire-hq": true,
	"build": true, "run": true, "update": true, "tag": true, "help": true,
}

// revertRoot finds the directory holding the executable `revert` dispatcher.
func revertRoot() string {
	if r := os.Getenv("REVERT_ROOT"); r != "" {
		return r
	}
	var cands []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands, d, filepath.Dir(d))
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands, wd, filepath.Dir(wd))
	}
	for _, c := range cands {
		if fi, err := os.Stat(filepath.Join(c, "revert")); err == nil && !fi.IsDir() {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
		}
	}
	wd, _ := os.Getwd()
	return wd
}

func main() {
	root := revertRoot()
	revert := filepath.Join(root, "revert")

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		fmt.Fprintln(os.Stderr, "embed error:", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/stream", streamHandler(root, revert))
	mux.HandleFunc("/api/info", infoHandler(root, revert))

	// Bind to loopback on a free port (local, single-user; never exposed).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	url := "http://" + ln.Addr().String()
	fmt.Printf("Revert GUI running at %s\n  toolkit root: %s\n(Ctrl-C to quit)\n", url, root)
	openBrowser(url)
	if err := http.Serve(ln, mux); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
	}
}

// infoHandler reports the toolkit root + whether the revert dispatcher was found.
func infoHandler(root, revert string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, statErr := os.Stat(revert)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"root":%q,"revertFound":%t}`, root, statErr == nil)
	}
}

// streamHandler runs `revert <cmd> [args...]` and streams merged output as SSE.
func streamHandler(root, revert string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		args := []string{cmd}
		args = append(args, r.URL.Query()["arg"]...) // repeated ?arg=... params, passed as separate argv

		send := func(event, data string) {
			for _, line := range strings.Split(data, "\n") {
				fmt.Fprintf(w, "event: %s\ndata: %s\n", event, line)
			}
			fmt.Fprint(w, "\n")
			flusher.Flush()
		}

		if _, err := os.Stat(revert); err != nil {
			send("error", "revert dispatcher not found at "+revert)
			send("done", "1")
			return
		}
		send("start", "revert "+strings.Join(args, " "))

		c := exec.Command(revert, args...)
		c.Dir = root
		c.Env = append(os.Environ(), "REVERT_ROOT="+root, "TERM=dumb")
		stdout, _ := c.StdoutPipe()
		c.Stderr = c.Stdout // merge stderr into the same stream
		if err := c.Start(); err != nil {
			send("error", err.Error())
			send("done", "1")
			return
		}
		scan := bufio.NewScanner(stdout)
		scan.Buffer(make([]byte, 64*1024), 1024*1024)
		for scan.Scan() {
			send("log", stripANSI(scan.Text()))
		}
		exit := "0"
		if err := c.Wait(); err != nil {
			exit = "1"
			if ee, ok := err.(*exec.ExitError); ok {
				exit = fmt.Sprintf("%d", ee.ExitCode())
			}
		}
		send("done", exit)
	}
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
