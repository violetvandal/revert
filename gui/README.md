# revert-gui

A tiny desktop front-end for the [Revert](../README.md) toolkit — a friendly face over
the `revert` CLI for people who'd rather click than type.

It serves a local web page and drives the `revert` lifecycle
(`doctor` · `setup` · `acquire-game-data` · `build` · `run` · `update`), streaming each
command's output **live** into a console via Server-Sent Events. The CLI is the seam:
the GUI adds no logic of its own.

## Run
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
MVP — the six lifecycle steps with a live-streaming console. Next: per-step prereq
gating (grey out Build until Setup+game data are ready), a native folder picker for the
game path, and packaging (a desktop launcher / installer bundle per OS).
