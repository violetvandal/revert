# Revert architecture

Hybrid: a **Go "build" core** owns producing the edition; **bash** owns launch +
system setup. One config (`revert.conf`) is the shared source of truth.

```
revert (bash dispatcher)
├── doctor                      read-only prerequisite checks
├── setup        -> share/setup/revert-setup.sh    Wine/DXVK/controller + prefixes
├── acquire-game-data -> share/setup/revert-acquire.sh   your THUG2 copy -> pristine
├── build        -> tools/thugkit/thugkit build  (Go core)  + Python CAS post-pass
├── run <lane>   -> share/run/revert-run.sh       GE-Proton launch, 3 lanes
└── tag          -> thugkit tag                    custom Create-A-Graphic tags
```

## The three planes
- **`thugkit` (Go, `tools/thugkit/`)** — the zero-dep, cross-platform builder. The
  `build` subcommand mirrors the pristine base, installs the no-CD exe, applies the
  WidescreenFix (the load-bearing `dinput8.dll` → **`winmm.dll`** rename, so Wine's
  native DirectInput stays for the controller), runs the mod apply (`apply.Run`),
  installs custom tags (`tag.Run`), copies the HUD-fix + glyph-fix `.asi`s, overlays the
  HQ A/V pack, and optionally bakes a default soundtrack. **No `os/exec`** — it never shells
  out, so the binary stays static. Reuses the existing `apply`, `tag`, `prx`, `imgxbx`,
  `grf` packages.
- **`share/run/revert-run.sh` (bash)** — GE-Proton launch, lanes defined entirely in
  `revert.conf` (`LANE_<NAME>_{DIR,PREFIX,EXE,ENV,HOOKS,SOUNDTRACK}`). Hooks: soundtrack
  swap (engine can't toggle live) and the evdev trigger bridge. Also resolves the button-glyph
  style (`GLYPH_STYLE` / `--glyphs`, Steam Deck auto-detect) into `$VV_GLYPHS` for the glyph `.asi`.

## Custom `.asi` mods (`tools/{hudfix,glyphfix}/`)
Two small hand-rolled `.asi`s (32-bit Windows DLLs, cross-compiled on Linux with mingw) loaded by
the Ultimate ASI Loader (our `winmm.dll`) alongside the stock WidescreenFix:
- **`VV.HudFix.asi`** — pulls the score / goal-points HUD to the true top-left on ultrawide.
- **`VV.GlyphFix.asi`** — renders trick-combo button prompts as controller glyphs instead of
  keyboard keys ("kp2"). Flips the font renderer's face-button branch (`0x4ced6f`/`0x4cff38`) and,
  for PlayStation/GameCube, repoints the buttons-font name to `ButtonsPs2`/`ButtonsNgc`. Style comes
  from `$VV_GLYPHS` (launcher) or the in-game **MOD OPTIONS → Button Glyphs** menu (read live from the
  GlobalFlag bitfield); keyboard↔controller is live, console art applies on the next launch.
  These hardcode addresses for the no-CD `THUG2.exe` (md5 `d464781a…`); re-derive if it changes.
- **`share/setup/revert-setup.sh` (bash)** — GE-Proton + DXVK + winetricks + `winmm`
  override + controller across the main (+ optional online) prefix.

## Why the split
Archive extraction (ISO/MSI/7z) and the optional **Python CAS asset steps**
(panty recolour, stickers, licensed decks/playas) are the **bash orchestrator's**
job — the Go core takes already-extracted directories. This keeps the Go binary
zero-dep and cross-platform while the build still reproduces the full edition.

## Cross-repo boundary (the one hard seam)
`tools/thugkit/` and `mods/` are **independent git repos**, gitignored by the root
**Revert** repo. The Go build code lives in the thugkit repo; the root repo holds the
bash orchestrator + shippable non-game assets + docs + config. They communicate ONLY
through the built `thugkit` binary's CLI — never a Go import from root.

- **Pinned thugkit commit:** `16424da` (`build` core). `revert doctor` checks the
  binary exposes `build`; `revert build` rebuilds it from source if a Go toolchain is
  present. A shipped Revert carries a prebuilt binary.

## No game data, ever
Every build input that is game data or licensed/derivative is **user-supplied and
gitignored**: the pristine base, no-CD exe, WidescreenFix zip, HQ A/V pack, and the
`mods/src/*/blob/` brand decks / guest models. The repo ships only the orchestrator,
the persona `.GRF` tag, the HUD-fix, the controller `.reg`s + bridge, and the mod
`.ns` sources.

## Deferred (not in v1)
Fyne GUI front door; Windows / Steam-Deck packaging; porting the Python CAS steps and
the licensed-asset injectors to Go; multi-distro setup beyond Fedora (doctor warns).

## Legacy (superseded, kept for reference)
`rebuild-playable.sh` (→ `revert build`), the `run-*.sh` / `launch-*.sh` zoo
(→ `revert run`), and `install.sh` (→ `revert setup` + `acquire-game-data`).
