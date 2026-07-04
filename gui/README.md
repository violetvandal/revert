# revert-gui

A tiny desktop front-end for the [Revert](../README.md) toolkit — a friendly face over
the `revert` CLI for people who'd rather click than type.

It serves a local web page and drives the `revert` lifecycle
(`doctor` · `setup` · `acquire-game-data` · `build` · `run` · `update`), streaming each
command's output **live** into a console via Server-Sent Events. The CLI is the seam:
the GUI adds no logic of its own.

## Run
The easy way — from the toolkit root:
```sh
./revert gui              # builds the binary once (needs Go), then launches
```
Or add a **click-to-launch app-menu entry** (no terminal afterwards):
```sh
./revert install-desktop  # "THUG2: Violet Vandal Edition" in your app menu
```
Or build/run it directly:
```sh
cd gui && go build -o revert-gui . && ./revert-gui
```
It picks a free loopback port, prints the URL, and opens your browser. It finds the
`revert` dispatcher automatically (parent dir), or set `REVERT_ROOT=/path/to/revert`.

## Why pure Go / web UI
- **Zero native dependencies** — stdlib only (`net/http` + `embed`), no CGO, no OpenGL/X11.
- **One static binary per OS**, trivially cross-compiled:
  ```sh
  GOOS=windows GOARCH=amd64 go build -o revert-gui.exe .
  GOOS=darwin  GOARCH=arm64 go build -o revert-gui .
  ```
- Bound to `127.0.0.1` only; runs `revert` subcommands from a fixed whitelist (never shell
  input). Single-user, local-only.

## Status
The six lifecycle steps with a live-streaming console, plus:

- **Prereq gating** — steps light up ✓ as they complete and stay locked until their
  prerequisites are met (Build waits on game data; Play waits on a build + Wine). Driven
  by `revert status --json`, re-polled after every command.
- **Native folder picker** — the *Browse…* button opens the desktop's own directory
  dialog (`zenity`/`kdialog` on Linux, `osascript` on macOS, PowerShell on Windows) and
  fills in the game path.
- **Desktop integration** — `revert gui` builds-and-launches; `revert install-desktop`
  (also run at the end of `revert setup`) adds an app-menu entry so it launches with no
  terminal. On the Steam Deck, add `revert` to Steam with launch options `gui`.

### Platform scope
Revert is a clone-based **Linux / Steam Deck** toolkit (bash dispatcher + GE-Proton Wine),
so packaging = desktop integration for those, not a self-contained installer. The Go GUI
binary cross-compiles to Windows/macOS, but the `revert` setup/run layer it drives is
Linux-only (and THUG2 runs natively on Windows anyway — no Wine, a different flow), so
those aren't end-to-end targets today.
