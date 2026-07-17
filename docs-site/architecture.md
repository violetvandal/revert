# Architecture

Revert is a hybrid: a **Go build core** owns producing the edition, and **bash** owns launch
and system setup. One config (`revert.conf`) is the shared source of truth. This page is the
map; the [build pipeline](build-pipeline.md) traces the core step by step.

## The dispatcher

`revert` (bash) is the front door. Its command surface:

```
revert (bash dispatcher)
├── doctor                 read-only prerequisite checks
├── setup            →  share/setup/revert-setup.sh     Wine/DXVK/controller + prefixes
├── acquire-game-data →  share/setup/revert-acquire.sh   your THUG2 copy → pristine base
├── acquire-hq       →  share/setup/revert-acquire-hq.sh fetch-or-BYO HQ audio/video packs
├── build            →  tools/thugkit/thugkit build      (Go core) + Python CAS post-pass
├── run <lane>       →  share/run/revert-run.sh          GE-Proton launch, three lanes
├── tag              →  thugkit tag                       custom Create-A-Graphic tags
├── gui / install-desktop                                 the graphical installer + app entry
├── calibrate-controller / configure-controller           pad setup
└── update / uninstall / status / version
```

Dispatch is a `case` at the bottom of `revert` routing to `cmd_*` functions. Configuration
lives in `revert.conf`: paths, the Wine runtime, and the `LANE_*` / `EDITION_*` variables.

## The three planes

### 1. `thugkit` (Go, `tools/thugkit/`)

The zero-dependency, cross-platform builder. Its `build` subcommand mirrors the pristine
base, installs the no-CD exe, applies the WidescreenFix, runs the mod apply, installs custom
tags and the HUD/glyph `.asi`s, overlays the HQ A/V pack, and optionally bakes a default
soundtrack. It **never shells out** (no `os/exec`), so the binary stays static and
cross-compiles for Windows, Linux, and the Deck. It reuses its own `apply`, `tag`, `prx`,
`imgxbx`, and `grf` packages. See [Codecs](codecs.md).

### 2. `share/run/revert-run.sh` (bash)

GE-Proton launch. Lanes are defined entirely in `revert.conf` as
`LANE_<NAME>_{DIR,PREFIX,EXE,ENV,HOOKS,SOUNDTRACK}`. Hooks handle the two things the engine
cannot do at build time: the live soundtrack swap and the evdev trigger bridge. It also
resolves the button-glyph style (`GLYPH_STYLE` / `--glyphs`, with Steam Deck auto-detect)
into `$VV_GLYPHS` for the glyph `.asi`. See [Platform lanes](platform-lanes.md).

### 3. `share/setup/revert-setup.sh` (bash)

GE-Proton + DXVK + winetricks + the `winmm` override + controller setup across the main
(and optional online) prefix.

### Why the split

Archive extraction (ISO/MSI/7z) and the optional **Python CAS asset steps** (part recolours,
stickers, licensed decks and guest models) are the bash orchestrator's job. The Go core
takes already-extracted directories. This keeps the Go binary zero-dependency and
cross-platform while the build still reproduces the full edition.

## Custom `.asi` mods (`tools/{hudfix,glyphfix}/`)

Two small hand-rolled `.asi`s (32-bit Windows DLLs, cross-compiled on Linux with mingw)
loaded by the Ultimate ASI Loader (our renamed `winmm.dll`) alongside the stock
WidescreenFix:

- **`VV.HudFix.asi`** pulls the score / goal-points HUD to the true top-left on ultrawide.
- **`VV.GlyphFix.asi`** renders trick-combo button prompts as controller glyphs instead of
  keyboard keys ("kp2"). It flips the font renderer's face-button branch and, for
  PlayStation / GameCube, repoints the buttons-font name. Style comes from `$VV_GLYPHS` or
  the in-game **MOD OPTIONS → Button Glyphs** menu (read live from a GlobalFlag bitfield);
  keyboard↔controller is live, console art applies on the next launch.

These hardcode addresses for the no-CD `THUG2.exe` (md5 `d464781a…`); re-derive them if it
changes. Both patch **cold in `DllMain`**, not from a worker thread, so they are sound under
Rosetta on macOS.

## The cross-repo boundary (the one hard seam)

`tools/thugkit/` and `mods/` are **independent git repos**, gitignored by the root Revert
repo. The Go build code lives in the thugkit repo; the root repo holds the bash orchestrator,
shippable non-game assets, docs, and config. They communicate **only** through the built
`thugkit` binary's CLI, never a Go import from root. `revert doctor` checks the binary
exposes `build`; `revert build` rebuilds it from source when a Go toolchain is present.

## The Windows lane (native, no Wine)

On Windows THUG2 runs natively, so the whole Wine plane collapses. A cross-platform Go front
door replaces bash there:

```
cmd/revert        revert.exe  — mirrors the bash subcommand surface
cmd/revert-gui    the same web UI, driving revert.exe
cmd/vv-padbridge  XInput → keystroke helper (the L2/R2 combos, native syscalls)
internal/core     conf parser + doctor/status/build/run/setup/acquire/soundtrack
```

The single rule that keeps the proven Linux/Deck path safe: **on Linux every core command
delegates to the bash dispatcher** (`internal/core/delegate.go`), so bash stays
authoritative; **on Windows the commands run natively**. The seams are unchanged: `build`
still shells to `thugkit`, `tag` passes through, `run` launches `THUG2.exe` directly.

## Legacy

`rebuild-playable.sh` (now `revert build`), the `run-*.sh` lane launchers (now `revert run`),
and the old `run-*-trace.sh` RE diagnostics have all been superseded and removed. The root
keeps only `install.sh`, the live one-command bootstrap that chains `revert setup` +
`acquire-game-data` + `build`.
