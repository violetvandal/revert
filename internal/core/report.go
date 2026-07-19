package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ReportOptions mirror `revert report [-o FILE] [--no-log]`.
type ReportOptions struct {
	Output string // where to write the report ("" -> ./revert-report.txt)
	NoLog  bool   // skip the tail of the last run's log
}

// Report collects a diagnostic bundle for a bug report: what hardware this is, what
// drivers it has, how far through the lifecycle the install got, and the tail of the
// last launch. It prints the report AND saves it, so it can be pasted into an issue or
// attached as a file.
//
// It is strictly read-only, and it redacts the home directory and username before
// anything leaves this function (see redactText). Someone pasting the result into a
// public issue should be handing over their GPU model, not their identity.
//
// On Linux the bash dispatcher owns this, for the same reason it owns doctor: the Wine
// runtime, the prefixes, the evdev bridges and the Deck detection all live there.
func Report(c *Conf, o ReportOptions) error {
	if IsLinux() {
		args := []string{}
		if o.Output != "" {
			args = append(args, "-o", o.Output)
		}
		if o.NoLog {
			args = append(args, "--no-log")
		}
		return DelegateToBash(c.Root, "report", args...)
	}

	out := o.Output
	if out == "" {
		wd, err := os.Getwd()
		if err != nil {
			wd = c.Root
		}
		out = filepath.Join(wd, "revert-report.txt")
	}

	body := redact(buildReport(c, o))
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		return fmt.Errorf("could not write the report to %s: %w", out, err)
	}

	fmt.Print(body)
	fmt.Printf("\n[revert] saved to %s\n", out)
	fmt.Println("Open an issue and attach it: https://github.com/violetvandal/revert/issues/new/choose")
	fmt.Println("Read it before you post if you like — it is plain text, and nothing in it is secret.")
	return nil
}

func buildReport(c *Conf, o ReportOptions) string {
	var b strings.Builder
	sec := func(title string) { fmt.Fprintf(&b, "\n== %s ==\n", title) }
	kv := func(k, v string) {
		if v == "" {
			v = "(unknown)"
		}
		fmt.Fprintf(&b, "%-22s %s\n", k, v)
	}
	yn := func(p string) string {
		if p != "" && (fileExists(p) || dirExists(p)) {
			return "yes"
		}
		return "no"
	}

	b.WriteString("Revert diagnostic report\n")
	b.WriteString("(paths and usernames redacted; safe to paste publicly)\n")

	sec("Revert")
	kv("version", Version)
	kv("root", c.Root)
	kv("platform", runtime.GOOS+"/"+runtime.GOARCH)

	sec("System")
	for _, p := range systemInfo() {
		kv(p.k, p.v)
	}

	sec("Graphics")
	kv("DXVK (configured)", c.GetOr("DXVK_VERSION", "unset"))
	for _, line := range gpuInfo() {
		b.WriteString("  " + line + "\n")
	}

	sec("Runtime")
	if IsMac() {
		wine := c.Path("MAC_WINE")
		kv("wine", wine)
		kv("wine present", yn(wine))
		kv("wine prefix", yn(c.Path("PREFIX_MAC")))
		if fileExists(wine) {
			kv("wine version", probe(wine, "--version"))
		}
	} else {
		kv("runtime", "native Windows (no Wine)")
		kv("DirectX 9 helper", map[bool]string{true: "present", false: "not detected"}[directXPresent()])
	}

	sec("Toolchain")
	kv("thugkit", yn(c.Thugkit()))
	kv("go", probe("go", "version"))
	if v := probe("python3", "--version"); v != "" {
		kv("python", v)
	} else {
		kv("python", probe("python", "--version"))
	}

	// Presence only. Which files exist tells us where the lifecycle stopped; their
	// contents are the user's own game data and are none of our business.
	sec("Game data (presence only)")
	kv("pristine base", yn(filepath.Join(c.Path("PRISTINE_DIR"), "Data", "pre")))
	kv("qol build", yn(filepath.Join(c.Path("EDITION_QOL"), "Data", "pre")))
	nocd := c.Path("NOCD_EXE")
	if fileExists(nocd) {
		// The shipped .asi mods hardcode addresses against one specific exe. A mismatch
		// here explains a whole family of "the HUD is in the wrong place" reports at a
		// glance, so it is worth the hash on every report.
		if sum, err := fileMD5(nocd); err == nil && sum == NoCDExeMD5 {
			kv("THUG2.exe", "md5 matches the expected no-CD exe")
		} else if err == nil {
			kv("THUG2.exe", "md5 "+sum+" (DIFFERENT from the expected no-CD exe)")
		}
	} else {
		kv("THUG2.exe", "not present")
	}

	if !o.NoLog {
		sec("Last run (tail)")
		log := RunLogPath()
		if data, err := os.ReadFile(log); err == nil {
			kv("log file", log)
			b.WriteString("\n")
			for _, line := range tailLines(string(data), 200) {
				b.WriteString("  " + line + "\n")
			}
		} else {
			b.WriteString("  No run log yet. Launch the game once (revert run qol), then re-run this.\n")
		}
	}

	return b.String()
}

type kvPair struct{ k, v string }

// systemInfo returns the OS/CPU/memory facts, per platform. A slice rather than a map so
// the field order is fixed and two reports diff cleanly against each other.
func systemInfo() []kvPair {
	switch runtime.GOOS {
	case "darwin":
		return []kvPair{
			{"macOS", probe("sw_vers", "-productVersion")},
			{"build", probe("sw_vers", "-buildVersion")},
			{"model", probe("sysctl", "-n", "hw.model")},
			{"cpu", probe("sysctl", "-n", "machdep.cpu.brand_string")},
			{"memory", probe("sh", "-c", "echo $(( $(sysctl -n hw.memsize) / 1073741824 )) GB")},
			// Rosetta matters: the Mac lane runs an x86 game under translation, and
			// "is this process translated" has already explained more than one report.
			{"translated", probe("sysctl", "-n", "sysctl.proc_translated")},
		}
	case "windows":
		return []kvPair{
			{"windows", psProbe(`(Get-CimInstance Win32_OperatingSystem).Caption`)},
			{"build", psProbe(`(Get-CimInstance Win32_OperatingSystem).BuildNumber`)},
			{"cpu", psProbe(`(Get-CimInstance Win32_Processor).Name`)},
			{"memory", psProbe(`[math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory/1GB).ToString() + " GB"`)},
		}
	}
	return nil
}

// gpuInfo returns one line per graphics adapter, with the driver version where the OS
// will tell us. The driver, not the card, is what usually decides whether this runs.
func gpuInfo() []string {
	switch runtime.GOOS {
	case "darwin":
		out := probeMulti("system_profiler", "SPDisplaysDataType")
		var keep []string
		for _, l := range strings.Split(out, "\n") {
			t := strings.TrimSpace(l)
			if strings.HasPrefix(t, "Chipset Model:") || strings.HasPrefix(t, "Metal") ||
				strings.HasPrefix(t, "VRAM") || strings.HasPrefix(t, "Vendor:") {
				keep = append(keep, t)
			}
		}
		if len(keep) == 0 {
			return []string{"(system_profiler returned nothing)"}
		}
		return keep
	case "windows":
		out := psProbeMulti(`Get-CimInstance Win32_VideoController | ForEach-Object { "$($_.Name) — driver $($_.DriverVersion)" }`)
		if strings.TrimSpace(out) == "" {
			return []string{"(no adapters reported)"}
		}
		return strings.Split(strings.TrimSpace(out), "\n")
	}
	return []string{"(not collected on this platform)"}
}

// RunLogPath is where the last launch's output is kept. It matches the path
// share/run/revert-run.sh writes on Linux, so a report reads the same file whichever
// front door produced it.
func RunLogPath() string {
	return filepath.Join(stateDir(), "last-run.log")
}

func stateDir() string {
	if v := os.Getenv("REVERT_STATE_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	switch runtime.GOOS {
	case "windows":
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "Revert")
		}
		return filepath.Join(home, "AppData", "Local", "Revert")
	case "darwin":
		return filepath.Join(home, "Library", "Logs", "Revert")
	default:
		if x := os.Getenv("XDG_STATE_HOME"); x != "" {
			return filepath.Join(x, "revert")
		}
		return filepath.Join(home, ".local", "state", "revert")
	}
}

// OpenRunLog creates (truncating) the last-run log and writes a header. A nil writer with
// a nil error means "could not log here" — callers pass it straight to runTee, which
// treats nil as "no log". Logging is a convenience; it must never block a launch.
func OpenRunLog(header string) io.WriteCloser {
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return nil
	}
	f, err := os.Create(RunLogPath())
	if err != nil {
		return nil
	}
	fmt.Fprintf(f, "# revert run — %s\n# ---- game output below ----\n", header)
	return f
}

// tailLines returns the last n lines of s.
func tailLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// redact strips the two things that identify a person: their home directory (which on
// every mainstream OS contains their login name) and the login name itself.
func redact(s string) string {
	home, _ := os.UserHomeDir()
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}
	return redactText(s, home, user)
}

// redactText is the testable core of redact. Home goes first and is matched
// case-insensitively (Windows paths vary in case, and both separators appear in the same
// report), so "/home/jane/x" becomes "~/x" rather than "/home/<user>/x".
//
// The username is matched on a word boundary and, unlike the home path, CASE-SENSITIVELY.
// Both restrictions exist because over-redaction is its own failure: a user called "ati"
// once turned every "HDA ATI HDMI" line into "HDA <user> HDMI", and a mangled report is
// worse than an unredacted one because it is wrong without looking wrong. Login names are
// case-sensitive on Linux and stably-cased within a single report on Windows, so matching
// exactly costs nothing real. Very short names are skipped for the same reason.
func redactText(s, home, user string) string {
	if home != "" {
		// Path case genuinely varies on Windows (C:\Users vs c:\users) and both separators
		// turn up in one report, so the home path is matched loosely on purpose.
		for _, h := range []string{home, strings.ReplaceAll(home, `\`, "/")} {
			if re, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(h)); err == nil {
				s = re.ReplaceAllString(s, "~")
			}
		}
	}
	if len(user) > 2 {
		if re, err := regexp.Compile(`\b` + regexp.QuoteMeta(user) + `\b`); err == nil {
			s = re.ReplaceAllString(s, "<user>")
		}
	}
	return s
}

// probe runs a command with a short timeout and returns its first line, or "" if the
// tool is missing or fails. Every probe is best-effort: a missing tool must degrade to a
// blank field, never abort the report, which is exactly when someone needs it most.
func probe(name string, args ...string) string {
	out := probeMulti(name, args...)
	if out == "" {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
}

func probeMulti(name string, args ...string) string {
	if _, err := exec.LookPath(name); err != nil && !filepath.IsAbs(name) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
}

// psProbe/psProbeMulti run a PowerShell one-liner. Windows has no stable CLI for hardware
// facts (wmic is deprecated and absent on recent builds), so CIM via PowerShell is the
// portable way to ask.
func psProbe(script string) string {
	a := powershell(script)
	return probe(a[0], a[1:]...)
}

func psProbeMulti(script string) string {
	a := powershell(script)
	return probeMulti(a[0], a[1:]...)
}

func powershell(script string) []string {
	return []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", script}
}
