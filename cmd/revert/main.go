// Command revert is the cross-platform front door for THUG2: Violet Vandal Edition.
//
// On Windows it is THE dispatcher — THUG2 runs natively, so it implements the whole
// lifecycle (doctor/status/setup/acquire/build/run) directly on top of the thugkit
// build core. On Linux/Steam Deck it is a thin pass-through to the proven bash `revert`
// dispatcher, so the validated Wine/Deck path stays authoritative. The GUI drives this
// same binary, giving one command surface on every platform.
package main

import (
	"fmt"
	"os"

	"github.com/violetvandal/revert/internal/core"
)

const usage = `revert — front door for THUG2: Violet Vandal Edition (Windows-native / Linux-delegating)

  revert doctor                              check prerequisites (read-only)
  revert setup [--online]                    native prereqs (DirectX 9, controller bindings)
  revert acquire-game-data --from <path>     turn YOUR THUG2 copy (folder or .zip) into the base
  revert build [--fast] [--lane qol|vanilla] [--only a,b]   build the edition
  revert run <vanilla|qol|online> [--soundtrack original|radio] [--glyphs xbox|playstation|gamecube|keyboard]
  revert status [--json]                     report lifecycle state (used by the GUI)
  revert calibrate-controller                detect the pad's DirectInput GUID + bind pad0
  revert install-desktop                     add Start Menu + Desktop shortcuts
  revert update [--check] [--force]          update to the latest release + rebuild
  revert uninstall [--dry-run] [--yes] [--purge]   remove Revert (--purge also removes saves, THUG Pro, Go, packages)
  revert tag <image> [...]                   make an in-game Create-A-Graphic tag
  revert gui                                 launch the click-to-install web UI
  revert version                             print the release this build came from
  revert help                                this help

NOTE: Revert ships TOOLING, never game data — you must own THUG2.`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println(usage)
		return
	}
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "help", "-h", "--help":
		fmt.Println(usage)
	case "version", "--version":
		fmt.Println(core.Version)
	case "doctor":
		run(withConf(func(c *core.Conf) error { return core.Doctor(c) }))
	case "status":
		run(withConf(func(c *core.Conf) error { return cmdStatus(c, rest) }))
	case "setup":
		run(withConf(func(c *core.Conf) error { return core.Setup(c, parseSetup(rest)) }))
	case "acquire-game-data":
		run(withConf(func(c *core.Conf) error { return core.Acquire(c, parseAcquire(rest)) }))
	case "build":
		run(withConf(func(c *core.Conf) error { return core.Build(c, parseBuild(rest)) }))
	case "run":
		run(withConf(func(c *core.Conf) error { return cmdRun(c, rest) }))
	case "tag":
		run(withConf(func(c *core.Conf) error { return core.Tag(c, rest) }))
	case "gui":
		run(withConf(func(c *core.Conf) error { return core.LaunchGUI(c, rest) }))
	case "calibrate-controller":
		run(withConf(func(c *core.Conf) error { return core.Calibrate(c) }))
	case "controls":
		// macOS "THUG2 Controls.app" launches this: THUG2's own Launcher.exe, the escape
		// hatch if a pad ever needs re-binding by hand.
		run(withConf(func(c *core.Conf) error { return core.Controls(c) }))
	case "install-desktop":
		run(withConf(func(c *core.Conf) error { return core.InstallDesktop(c) }))
	case "update":
		run(withConf(func(c *core.Conf) error { return core.Update(c, parseUpdate(rest)) }))
	case "uninstall":
		run(withConf(func(c *core.Conf) error { return core.Uninstall(c, parseUninstall(rest)) }))
	case "acquire-hq", "build-installer":
		// Not part of the native-Windows lane; on Linux hand straight to the bash dispatcher.
		run(withConf(func(c *core.Conf) error { return core.DelegateOrUnsupported(c, cmd, rest) }))
	default:
		fmt.Fprintf(os.Stderr, "revert: unknown command %q\n\n%s\n", cmd, usage)
		os.Exit(1)
	}
}

// withConf loads revert.conf and runs fn with it.
func withConf(fn func(*core.Conf) error) func() error {
	return func() error {
		c, err := core.LoadRootConf()
		if err != nil {
			return err
		}
		return fn(c)
	}
}

// run executes fn and exits non-zero (propagating a child exit code) on error.
func run(fn func() error) {
	if err := fn(); err != nil {
		code := core.ExitCode(err)
		if code == 0 {
			code = 1
		}
		// An *exec.ExitError already printed the child's own stderr; only print our own
		// orchestration errors.
		if _, isExit := isExitErr(err); !isExit {
			fmt.Fprintf(os.Stderr, "revert: %v\n", err)
		}
		os.Exit(code)
	}
}

func cmdStatus(c *core.Conf, rest []string) error {
	if len(rest) > 0 && rest[0] == "--json" {
		if core.IsNative() {
			core.StatusJSON(c)
			return nil
		}
		return core.DelegateToBash(c.Root, "status", "--json")
	}
	if !core.IsNative() {
		return core.DelegateToBash(c.Root, "status")
	}
	return core.Doctor(c)
}

func cmdRun(c *core.Conf, rest []string) error {
	if len(rest) == 0 {
		return fmt.Errorf("usage: revert run <vanilla|qol|online> [--soundtrack ..] [--glyphs ..]")
	}
	o := core.RunOptions{Lane: rest[0]}
	rest = rest[1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--soundtrack":
			o.Soundtrack, i = next(rest, i)
		case "--glyphs":
			o.Glyphs, i = next(rest, i)
		case "--":
			o.ExtraArgs = append(o.ExtraArgs, rest[i+1:]...)
			i = len(rest)
		default:
			o.ExtraArgs = append(o.ExtraArgs, rest[i])
		}
	}
	return core.Run(c, o)
}

func parseUpdate(rest []string) core.UpdateOptions {
	var o core.UpdateOptions
	for _, a := range rest {
		switch a {
		case "--check":
			o.Check = true
		case "--force":
			o.Force = true
		}
	}
	return o
}

func parseUninstall(rest []string) core.UninstallOptions {
	var o core.UninstallOptions
	for _, a := range rest {
		switch a {
		case "--dry-run":
			o.DryRun = true
		case "--yes", "-y":
			o.Yes = true
		case "--purge":
			o.Purge = true
		}
	}
	return o
}

func parseSetup(rest []string) core.SetupOptions {
	var o core.SetupOptions
	for _, a := range rest {
		switch a {
		case "--online":
			o.Online = true
		case "--online-only":
			o.OnlineOnly = true
		}
	}
	return o
}

func parseAcquire(rest []string) core.AcquireOptions {
	var o core.AcquireOptions
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--from", "--folder":
			o.From, i = next(rest, i)
		case "--url":
			o.URL, i = next(rest, i)
		}
	}
	return o
}

func parseBuild(rest []string) core.BuildOptions {
	o := core.BuildOptions{Lane: "qol"}
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--fast":
			o.Fast = true
		case "--lane":
			o.Lane, i = next(rest, i)
		case "--only":
			o.Only, i = next(rest, i)
		}
	}
	return o
}

// next returns rest[i+1] and the advanced index, or "" if there's no following arg.
func next(rest []string, i int) (string, int) {
	if i+1 < len(rest) {
		return rest[i+1], i + 1
	}
	return "", i
}

func isExitErr(err error) (int, bool) {
	type exitCoder interface{ ExitCode() int }
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode(), true
	}
	return 0, false
}
