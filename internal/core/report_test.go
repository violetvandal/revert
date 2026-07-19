package core

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// runTee must capture the child's output WITHOUT swallowing its exit code. Getting this
// wrong is the classic tee bug (a shell pipeline reports tee's status, so every crash
// looks like a clean exit) and it would make the run log actively misleading.
func TestRunTeeCapturesOutputAndPreservesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	var buf bytes.Buffer
	err := runTee(&buf, "", nil, "sh", "-c", `echo out; echo err >&2; exit 42`)
	if got := ExitCode(err); got != 42 {
		t.Errorf("exit code = %d, want 42", got)
	}
	for _, want := range []string{"out", "err"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log is missing %q; got %q", want, buf.String())
		}
	}
}

// A nil writer means "could not open the log", and must degrade to a normal run rather
// than refusing to launch the game.
func TestRunTeeWithNilWriterStillRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	if err := runTee(nil, "", nil, "sh", "-c", "exit 0"); err != nil {
		t.Errorf("nil log writer should still run: %v", err)
	}
}

// The report is written to be pasted into a public issue, so redaction is the one part
// of it that has to be right. These tests are the contract.
func TestRedactTextStripsIdentity(t *testing.T) {
	cases := []struct {
		name, in, home, user, want string
	}{
		{
			name: "linux home becomes tilde",
			in:   "GE_DIR /home/jane/.local/share/lutris",
			home: "/home/jane", user: "jane",
			want: "GE_DIR ~/.local/share/lutris",
		},
		{
			name: "windows home, case-insensitive",
			in:   `root c:\users\Jane\Documents\revert`,
			home: `C:\Users\Jane`, user: "Jane",
			want: `root ~\Documents\revert`,
		},
		{
			name: "windows home written with forward slashes",
			in:   "root C:/Users/Jane/revert",
			home: `C:\Users\Jane`, user: "Jane",
			want: "root ~/revert",
		},
		{
			name: "bare username elsewhere in the text",
			in:   "session owned by jane",
			home: "/home/jane", user: "jane",
			want: "session owned by <user>",
		},
		{
			name: "home wins over username, so no /home/<user>",
			in:   "path /home/jane/x",
			home: "/home/jane", user: "jane",
			want: "path ~/x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactText(tc.in, tc.home, tc.user); got != tc.want {
				t.Errorf("redactText()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// A short login name must not be substring-matched, or it corrupts unrelated hardware
// lines. A wrong report is worse than an unredacted one, because it does not look wrong.
func TestRedactTextDoesNotManglePartialMatches(t *testing.T) {
	in := "HDA ATI HDMI / Radeon RX 580"
	got := redactText(in, "/home/ati", "ati")
	if got != in {
		t.Errorf("substring of a hardware name was redacted:\n got: %q\nwant: %q", got, in)
	}
}

// A very short username matches almost everything; refuse rather than shred the report.
func TestRedactTextIgnoresTinyUsernames(t *testing.T) {
	in := "a quick brown fox"
	if got := redactText(in, "", "a"); got != in {
		t.Errorf("one-character username was applied: %q", got)
	}
}

// Username matching is case-sensitive on purpose (see redactText): the loose version
// mangled hardware names. This pins the behaviour so nobody "fixes" it back.
func TestRedactTextUsernameIsCaseSensitive(t *testing.T) {
	in := "vendor ATI, user jane"
	want := "vendor ATI, user <user>"
	if got := redactText(in, "", "jane"); got != want {
		t.Errorf("redactText()\n got: %q\nwant: %q", got, want)
	}
}

func TestRedactTextHandlesEmptyInputs(t *testing.T) {
	if got := redactText("nothing to do", "", ""); got != "nothing to do" {
		t.Errorf("empty home/user changed the text: %q", got)
	}
}

func TestTailLines(t *testing.T) {
	if got := tailLines("a\nb\nc\nd\n", 2); strings.Join(got, ",") != "c,d" {
		t.Errorf("tailLines(4 lines, 2) = %v", got)
	}
	// Fewer lines than requested must return them all, not pad or panic.
	if got := tailLines("only\n", 10); strings.Join(got, ",") != "only" {
		t.Errorf("tailLines(1 line, 10) = %v", got)
	}
	if got := tailLines("", 5); strings.Join(got, ",") != "" {
		t.Errorf("tailLines(empty) = %v", got)
	}
}

// RunLogPath must honour the override the bash lane and the tests both rely on, so one
// report reads the log the other front door wrote.
func TestRunLogPathHonoursStateDirOverride(t *testing.T) {
	t.Setenv("REVERT_STATE_DIR", "/tmp/revert-state-test")
	if got := RunLogPath(); got != "/tmp/revert-state-test/last-run.log" {
		t.Errorf("RunLogPath() = %q", got)
	}
}
