package core

import (
	"path/filepath"
	"strings"
	"testing"
)

// buildReport is the Windows/macOS report body. On Linux, Report() delegates to the bash
// dispatcher, so this function never executes on the machine most of the development
// happens on. That is exactly why it needs a smoke test: a panic, a nil deref or a bad
// format verb would otherwise surface for the first time on a user's machine, at the
// moment they were already trying to report a different problem.
//
// This does not check the platform probes (they return "" off their own OS). It checks
// that the thing assembles and is shaped like a report.
func TestBuildReportDoesNotPanic(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// Point the loader at the repo root explicitly. Without this the test silently SKIPS
	// when run from this package's directory, which would make it look green while never
	// having executed the code it exists to cover.
	t.Setenv("REVERT_ROOT", root)

	c, err := LoadRootConf()
	if err != nil {
		t.Fatalf("could not load revert.conf from %s: %v", root, err)
	}

	out := buildReport(c, ReportOptions{NoLog: true})

	for _, want := range []string{
		"Revert diagnostic report",
		"== Revert ==",
		"== Toolchain ==",
		"== Game data (presence only) ==",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing section %q", want)
		}
	}
	// "%!d(string=...)" and friends mean a Printf verb does not match its argument.
	if strings.Contains(out, "%!") {
		t.Errorf("report contains a bad format verb:\n%s", out)
	}
	// NoLog must actually suppress the log section, or a report taken with --no-log still
	// carries the tail someone opted out of sending.
	if strings.Contains(out, "Last run (tail)") {
		t.Error("NoLog:true still emitted the run-log section")
	}
}
