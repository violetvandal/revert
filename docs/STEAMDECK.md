# THUG2: Violet Vandal Edition — Steam Deck

The Deck is the flagship target, and the install is **turnkey** — full analog controller,
widescreen, and the QOL mods, all automatic. There are two ways in: the **download-and-click
installer** (easiest, no terminal, just the Deck) or a **build-on-a-PC-and-sync** flow (for
people already running Revert on a Linux PC).

Everything Deck-specific is handled automatically by detecting the hardware
(DMI board `Jupiter`/`Galileo`). You do not hand-edit anything.

---

## Easiest: the download-and-click installer

No terminal and no second computer — just the Deck.

1. In **Desktop Mode**, open Firefox and download the installer:
   **[`revert-installer-linux-amd64`](https://github.com/violetvandal/revert/releases/latest)**
   (under *Assets* on the latest release).
2. In Dolphin, right-click it → **Properties → Permissions → Is executable**, then double-click it.
3. A wizard opens in your browser. Give it three things — where to install, a password (it
   **creates your Deck password** if you don't have one yet), and a link or folder for your own
   THUG2 copy — then press **Install & build**. It installs Wine, fetches your game, builds the
   edition, and calibrates the controller, keeping the Deck awake the whole time. No terminal at
   any point.
4. When it finishes, it tells you to **switch to Gaming Mode** and find **"Tony Hawk's Underground
   2 (VV Edition)"** under your Non-Steam games. Launch it and play.

---

## Advanced: build on a PC and sync

If you already run Revert on a Linux PC, build there and push to the Deck:
```sh
./revert build                              # build the QOL edition (game-playable-us)
tools/deck/sync-to-deck.sh                  # rsync it + the toolkit to deck@<ip>:thug2
```
Then **on the Deck, in Desktop Mode** — double-click **`tools/deck/Install-THUG2-Violet-Vandal.desktop`** in Dolphin (or run `cd ~/thug2 && ./revert setup` in Konsole). **Keep Steam open** so the Steam on-screen keyboard is available for the one sudo prompt; this manual setup closes and reopens Steam itself when it writes the shortcut. (The graphical installer above instead leaves Steam closed and hands you straight back to Gaming Mode.)

Then **switch to Gaming Mode and launch "Tony Hawk's Underground 2 (VV Edition)"** from
your library — it shows up with a proper cover/hero/logo, not a blank tile. That's it.

---

## What `revert setup` does on the Deck

All automatic, idempotent (re-runnable), and Deck-gated:

1. **Wine 11.11 (Kron4ek).** `wine-ge-8-26`'s 2023 build crashes *every* 32-bit app on
   current SteamOS (glibc 2.41) within seconds, so the Deck needs a current wine.
   Setup extracts the bundled `tools/wine-11.11-staging-amd64.tar.xz` (shipped by
   `sync-to-deck.sh`), or downloads it if absent.
2. **32-bit X libs** via `pacman` (`lib32-libxrender`, `…xcursor`, `…xi`, `…xrandr`,
   `…xcomposite`, `…xkbcommon`). Without these the game can't even open a window
   (`nodrv_CreateWindow`). This is the one step that needs **sudo** (it toggles
   SteamOS read-only off and inits the pacman keyring). Already-present → skipped.
3. **A win32 wine prefix** (`~/.wine-thug2-ge`) + **DXVK** + a **1280×800 virtual
   desktop** + the **WSFix `winmm` proxy**.
4. **The controller** — imports the Deck button map and starts the pad-mirror to
   auto-detect `pad0` (see below).
5. **The Steam shortcut + library artwork** — writes "Tony Hawk's Underground 2 (VV
   Edition)" → `play-qol.sh` straight into Steam's `shortcuts.vdf` (round-trip-validated,
   backed up), and installs the cover/hero/logo/icon from `tools/deck/art/` into Steam's
   `userdata/.../config/grid/` keyed to the shortcut's appid — so the library entry looks
   like a real game instead of a blank tile. Any earlier "THUG2: Violet Vandal Edition"
   shortcut is removed so there's no duplicate. Since this must happen with Steam *closed*
   but Steam is also the Deck's keyboard, setup runs it last (after the password step) and
   **cleanly `steam -shutdown`s, writes it, and relaunches Steam** automatically. (Run
   setup in **Desktop Mode** — it won't do this in Gaming Mode, where closing Steam would
   end the session; there it just tells you to add it via *Add a Non-Steam Game* →
   `~/thug2/play-qol.sh`.)

> **Play in Gaming Mode.** Desktop Mode + Big Picture loses Steam-Input focus on level
> loads. Gaming Mode (gamescope) holds the game in focus and is rock-solid.

---

## The controller — full analog, never drops

You don't configure anything in Steam Input. The base Steam Input "Gamepad" template
(the default for a non-Steam game) is all that's needed; everything else is handled by
a small background bridge that `revert run` starts and stops automatically.

### Layout (PS2-faithful)
| Deck control          | THUG2 action                           |
|-----------------------|----------------------------------------|
| Left stick (analog)   | Skate / move                           |
| Right stick (analog)  | Camera                                 |
| A / B / X / Y         | Ollie / Grab / Flip / Grind            |
| D-pad                 | Park editor / menus                    |
| L1 / R1               | Spin left / right                      |
| **L1 + R1**           | **Get off board / walk**               |
| L2 / R2               | Nollie / Switch (R2 alone = acid drop) |
| **L2 + R2**           | **Level out**                          |
| L3 / R3               | Focus / camera toggle                  |
| Start (☰)             | Pause                                  |

### How it works (and why)
THUG2 reads its gamepad over Wine DirectInput. On the Deck the pad is *Steam Input's
emulated Xbox controller*, and Wine's DirectInput state for that pad **intermittently
stalls** mid-game (after a level load, an idle period, or heavy trigger use) — the
device keeps streaming at the OS level but Wine stops handing the live state to the
game, so analog input dies. This is a Wine-pipeline quirk specific to Steam's emulated
pad; binding/focus/GUID tweaks don't fix it.

**The fix:** `tools/trigger-bridge/thug2-pad-mirror.py` creates one *persistent* virtual
analog gamepad — **"Violet Vandal Pad"** (a non-Xbox device, so Wine reads it as a plain
DirectInput HID pad with no flaky XInput wrapper) — and continuously mirrors Steam's
emulated pad into it, following Steam's pad even if it migrates or is recreated. THUG2
binds to **our** stable pad (via the `pad0` registry value), so Steam's pad can
stall/recreate/idle all it wants and the game never notices. Result: **full analog that
never drops.** The 2-button combos THUG2 can't bind natively (L1+R1 get-off, etc.) are
emitted as keystrokes on a second virtual device.

The bridge is dependency-free (Python stdlib only — no `python3-evdev`), reads the pad
without grabbing it, and dies with the game.

---

## Glyphs & resolution
- **Glyphs**: auto → **Xbox** on the Deck. Change per-launch with
  `run qol --glyphs <xbox|playstation|gamecube|keyboard>` or in-game via
  *Game Options → MOD OPTIONS → Button Glyphs*.
- **Resolution**: native **1280×800**. The HUD fix is resolution-independent, so the
  score/goal HUD lands correctly with no tuning.

---

## Troubleshooting
- **No controller in-game** — make sure you launched *through Steam* (Steam Input only
  applies to games launched from the library). If still dead, the prefix's `pad0` may
  not match this prefix's virtual pad: re-run `./revert setup` (with the game closed)
  to re-detect it.
- **Black screen on boot / app dies in seconds** — wrong wine. `GE_DIR` must point at
  `wine-11.11-staging-amd64`, not `wine-ge-8-26`. `revert.conf` selects this per-host
  automatically; confirm `wine notepad` stays alive in the prefix.
- **Widescreen missing** — make sure `game-playable-us/scripts/*WidescreenFix.asi` made
  it across (the rsync whitelist includes it; an over-broad `Tony*` exclude once dropped
  it).
- **Input dropped once after editing Steam controller settings** — opening the Steam
  Input config UI while in-game resets the pad; just relaunch.
- **Don't run wine against the prefix while the game is running** — it causes wineserver
  contention that can hang or break live input.
- **Online lane (THUG Pro)** — `run online` works the same way; THUG Pro has its own
  native pad binding, so the mirror bridge isn't used there.

---

## Recipe notes (for maintainers)
- Transfer is a **whitelist** (`sync-to-deck.sh`): only the playable build + the bits
  setup/run need (~2.6 GB), never the dev machine's disc rips/backups.
- The Deck pad's DirectInput `guidInstance` is **per-prefix** and only deterministic
  when our virtual pad is the *sole* one in a *fresh* wineserver — the launch hook and
  setup both ensure that (kill stale bridges; detect against a clean wineserver).
- Full root-cause + diagnosis history lives in the project memory
  (`project_steamdeck_controller`, `project_steamdeck_lane`).
