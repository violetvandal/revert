# Installing THUG2: Violet Vandal Edition with Revert

Revert ships **tooling, not game data**. You must own Tony Hawk's Underground 2 (PC).
Some optional enhancements are user-supplied (see below).

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
- **HQ Xbox A/V pack** (`HQ_AUDIO_PACK`) — optional higher-quality music/video.
- Licensed brand decks / guest models live in gitignored `mods/src/*/blob/` (dev-only).

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

## Troubleshooting
- `./revert doctor` first — it pinpoints missing deps/prefixes/inputs.
- Controller has no L2/R2/walk: ensure `python3-evdev` + `/dev/uinput` access (doctor warns).
- Black screen after a mod change: the boot-safety ceiling on `qb_scripts.prx` is enforced
  by the builder; if you hand-edit mods, rebuild with `./revert build --fast`.
