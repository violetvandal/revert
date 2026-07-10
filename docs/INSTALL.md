# Installing THUG2: Violet Vandal Edition with Revert

Revert ships **tooling, not game data**. You must own Tony Hawk's Underground 2 (PC).
Some optional enhancements are user-supplied (see below).

> **Just want the easy path?** Download the graphical installer
> ([`revert-installer-linux-amd64`](https://github.com/violetvandal/revert/releases/latest))
> or run the one-command bootstrap (`bash <(curl -fsSL …/install.sh)`) — both do everything
> below for you. This document is the **manual / CLI** reference for driving each step yourself.

## Prerequisites
- Linux (Fedora is the tested/flagship target; other distros: install the equivalents).
- A **GE-Proton / wine-ge** runner (via Lutris or ProtonUp-Qt). System Fedora wine is
  wow64-only and cannot host the win32 prefix THUG2 needs. Point `GE_DIR` in
  `revert.conf` at it.
- Packages: `winetricks p7zip p7zip-plugins msitools cabextract python3-evdev`
  (`revert setup` installs these on Fedora).
- A Go toolchain (only to build `tools/thugkit/thugkit` from source; shipped builds
  carry a prebuilt binary).

Run `./revert doctor` at any time — it reports exactly what's present and what's next.

## 1. System setup (once)
```sh
./revert setup            # Fedora deps, GE prefix, DXVK, winetricks, winmm override, controller
./revert setup --online   # also prepare the THUG Pro (online) prefix
```
Idempotent — existing Wine prefixes are reused. The controller's L2/R2 trigger bridge
needs write access to `/dev/uinput`; `revert setup` prints the udev/group commands if
it isn't writable.

## 2. Provide the game (your own copy)
```sh
./revert acquire-game-data --folder /path/to/an/installed/THUG2   # Steam/GOG/disc install
# or from discs:
./revert acquire-game-data --iso CD1.iso --iso CD2.iso --iso CD3.iso
./revert acquire-game-data --disc-dir /path/with/the/ISOs
```
This produces the clean **pristine base** (`game-pristine-us/`) that builds derive from.

### User-supplied extras (optional, not shipped)
Place these where `revert.conf` points, then build:
- **no-CD executable** (`NOCD_EXE`) — the disc exe won't run under Wine (SafeDisc).
- **WidescreenFix zip** (`WSFIX_ZIP`) — ThirteenAG's THUG2 WidescreenFix release.
- Licensed brand decks / guest models live in gitignored `mods/src/*/blob/` (dev-only).

### HQ packs — `revert acquire-hq`
Two optional community/derivative packs sharpen the edition; Revert **does not host**
them (they're game-derivative), but makes them easy to pull once you point it at a source:

- **HQ Xbox audio/video** — higher-quality music + cutscene audio ripped from the Xbox release.
- **HQ classic level textures** (CA/DJ/SC) — sharper level art.

```sh
./revert acquire-hq            # fetch both (or: audio | textures)
```
How it resolves each pack:
1. If you set its URL in `revert.conf` (`HQ_AUDIO_URL` / `HQ_TEXTURES_URL`, plus an optional
   `_SHA256` to verify), `acquire-hq` downloads it to the right place.
2. If the URL is empty, it prints exactly where to **drop your own copy** — then you're done.

Either way, the next `revert build` applies them automatically. The HQ-textures mod is
`optional=true`, so a build simply **skips** it when the textures aren't present — nothing
breaks if you don't have them.

## 3. Build the edition
```sh
./revert build            # full build (base + no-CD + widescreen + mods + tags + HQ A/V)
./revert build --fast     # quick rebuild — resets Data/pre + re-applies mods only
./revert build --only mod-options-menu   # iterate one mod
```
The build is **fully reproducible**: everything is derived from the tracked sources +
your supplied files. Re-running produces the same edition.

## 4. Play
```sh
./revert run qol                      # QOL-Modded (flagship)
./revert run qol --soundtrack radio   # royalty-free "Violet Vandal Radio" (stream-safe)
./revert run vanilla                  # clean THUG2 + widescreen + controller
./revert run online                   # THUG Pro (after: ./revert setup --online)
```

## 5. Updating
```sh
./revert update --check   # is a newer release available?
./revert update           # update to the latest release + rebuild
```
`revert update` fetches the latest tagged release, moves the toolkit **and its pinned
components** (thugkit + mods + NeverScript submodules) to that version, rebuilds thugkit,
and re-runs `revert build` — **preserving your `Save/`** and never touching game data.

Keep machine-specific settings (e.g. `HQ_*_URL`, `GLYPH_STYLE`, a custom `GE_DIR`) in a
**`revert.conf.local`** file next to `revert.conf`. It's gitignored, sourced last (so it
wins), and never conflicts on update — edit `revert.conf` directly only if you want to
change a default for everyone.

## 6. Uninstalling
```sh
./revert uninstall            # preview the plan, then confirm
./revert uninstall --dry-run  # show what would go, remove nothing
./revert uninstall --purge    # full clean (see below)
```
The default removes the toolkit, the built editions, the Wine prefixes, the shortcuts and
the controller bindings — **after backing up every save and created tag** to a dated
`~/thug2-saves-backup-<date>/` folder. It **keeps** your saves' backup, the Go build tool,
THUG Pro, and any shared system libraries.

`--purge` is the full clean: it additionally deletes your saves **with no backup**, removes
THUG Pro, and removes the Go toolchain and system packages **only if Revert installed them**
(a Go or a library you already had is left alone). A GUI **Uninstall** button offers the same
two depths behind a typed confirmation.

## Troubleshooting
- `./revert doctor` first — it pinpoints missing deps/prefixes/inputs.
- Controller has no L2/R2/walk: ensure `python3-evdev` + `/dev/uinput` access (doctor warns).
- Black screen after a mod change: the boot-safety ceiling on `qb_scripts.prx` is enforced
  by the builder; if you hand-edit mods, rebuild with `./revert build --fast`.
