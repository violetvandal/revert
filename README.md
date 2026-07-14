# Revert — THUG2: Violet Vandal Edition toolkit

**Revert** builds and runs the *definitive, modernized Tony Hawk's Underground 2* on
**Linux / Steam Deck, Windows, and macOS**. It ships **tooling, never game data**, so you
bring your own THUG2 copy.

One front door, three lanes:

| Lane | What |
|------|------|
| **Vanilla** | clean THUG2 + no-CD + widescreen + controller |
| **QOL-Modded** *(flagship)* | the curated mod suite (MOD OPTIONS / LEVEL MODS, HUD fix, HQ A/V, custom tags…) — every mod default-off, vanilla one click away |
| **Online** | THUG Pro (bundled optional companion, isolated prefix) |

## Install (one command)
On a **Steam Deck**, any **Linux** desktop (Fedora, Arch, Ubuntu…), or a **Mac**, paste
this into a terminal:
```sh
bash <(curl -fsSL https://raw.githubusercontent.com/violetvandal/revert/main/install.sh)
```
That's the whole install. It fetches the toolkit (+ submodules), installs the Go build
tool locally if missing, runs system setup (Wine, controller, and on a Deck a Steam
shortcut), downloads *your* THUG2 copy from a link you paste, and builds the edition.
On a fresh Steam Deck the result is **turnkey**: it lands in your library and plays with
widescreen + analog controller, no manual steps. See [docs/STEAMDECK.md](docs/STEAMDECK.md).

> Run it exactly as shown — `bash <(curl …)`, **not** `curl … | bash`. Piping makes the
> script bash's stdin, so the one-time password/sudo prompt can't read your keyboard.

The installer clones into `~/thug2` and symlinks `revert` into `~/.local/bin`, so after
setup you can run `revert <cmd>` from anywhere (the examples below drop the `./`).

### …or click, don't type
Prefer not to touch a terminal? **Download the installer** and run it — a graphical
wizard does everything the command above does, with a live progress log. It asks for just
three things (where to install, your account password, and a link/folder for your THUG2
copy), then sets up Wine, fetches your game, and builds the edition. On a fresh Steam Deck
it even sets your account password for you and handles the one `sudo` step with no
terminal. Grab the installer for your machine from the
[latest release](https://github.com/violetvandal/revert/releases/latest):

| Machine | Download |
|---------|----------|
| Linux / Steam Deck | `revert-installer-linux-amd64` |
| Mac (Apple Silicon) | `revert-installer-darwin-arm64` |
| Mac (Intel) | `revert-installer-darwin-amd64` |
| Windows | `revert-windows-amd64.zip`, then run `revert-gui.exe` |

On Linux, mark it executable (right-click → *Properties → Permissions → Is executable*, or
`chmod +x`) and double-click it. On a Mac, see the Gatekeeper note below.

### On a Mac
Both **Apple Silicon** (M1 and later) and **Intel** Macs are supported, with the same
one-command install above. THUG2 is a 32-bit Direct3D 9 game from 2004, so Revert runs it
under Wine and translates its graphics to Metal through a build of DXVK we patched
specifically for Macs, whose Metal backend has no geometry shaders. The result is
GPU-accelerated, not a software-rendered slideshow.

The install needs **no Homebrew and no admin password**. It downloads a checksum-verified
Wine into the toolkit folder and installs nothing system-wide. macOS will ask you to
install Apple's Command Line Tools if you don't already have them, which is a normal
system dialog and the installer waits for it.

When it finishes, launch **"THUG2 Violet Vandal Edition"** from `~/Applications` or
Spotlight, like any other app, or run `revert run qol`.

For a controller, pair an **Xbox pad in XInput mode**. macOS only exposes
Microsoft-vendor pads to Wine, so other brands will pair with macOS but stay invisible to
the game.

> If you download the graphical installer from the releases page instead of using the
> one-line command, macOS will quarantine it because it is unsigned. Right-click the file
> and choose *Open*, then confirm, which tells Gatekeeper you trust it. The one-line
> install above is not affected.

## Quick start (already cloned)
If you cloned the repo yourself (`git clone --recursive`), drive the lifecycle directly:
```sh
revert doctor                         # check prerequisites
revert status                         # what's done vs. still needed (add --json for tools)
revert setup                          # one-time Wine/DXVK/controller + prefixes + launcher
revert acquire-game-data --folder /path/to/your/THUG2   # your copy -> pristine base
revert build                          # build the edition (reproducible)
revert run qol                        # play  (also: vanilla | online)
```
No installed folder to point at? `revert acquire-game-data --url <link>` downloads a
`.zip`/`.7z`/`.iso`/`.tar.*` you supply, and `--iso <cd1> [--iso cd2 …]` builds from disc
images. Revert provides no game data and no sources — `--url` is just a downloader.

`revert run qol --soundtrack radio` plays the royalty-free "Violet Vandal Radio"
soundtrack (stream-safe). `revert tag <image>` imports a custom Create-A-Graphic tag.
`revert run qol --glyphs playstation` themes the on-screen trick-combo button prompts
(Xbox / PlayStation / GameCube / keyboard; `auto` picks Xbox on Steam Deck). You can also
change it in-game under **Game Options → MOD OPTIONS → Button Glyphs**.

On a **Steam Deck**, if the controller ever comes up unbound, run
`revert calibrate-controller` once — it detects this prefix's gamepad and binds it. (The
one-command install runs this for you.)

**Naming a skater without a keyboard?** THUG2's PC text-entry screens (create-a-skater
name, save-game names, arcade high-score initials) are keyboard-only — a problem on a
Steam Deck. Revert adds **controller text entry** that turns on automatically when a text
field is open: the **D-pad or left stick** cycles the letter, **A** commits it, **X**
backspaces, and **Start** saves. (On a keyboard the same maps to **F5**/**F6** cycle,
**F7** commit, **F8** backspace, **F10** done.)

**Prefer clicking to typing?** Run `revert gui` for a small local web UI that runs the
same lifecycle (doctor/setup/build/run/update) with a live output console. `revert setup`
also installs an app-menu launcher ("THUG2: Violet Vandal Edition") — or add one anytime
with `revert install-desktop` — so you never need a terminal again. See
[gui/README.md](gui/README.md).

See [docs/INSTALL.md](docs/INSTALL.md) for the full setup,
[docs/STEAMDECK.md](docs/STEAMDECK.md) for the Steam Deck lane,
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how it's built.

## What's in the box
- `install.sh` — one-command bootstrap (clone + setup + acquire + build); see [Install](#install-one-command)
- `revert` — the dispatcher (this is the only command you run)
- `revert.conf` — single source of truth (paths, wine runtime, lanes)
- `share/` — bash planes (`run/`, `setup/`) + shippable assets (controller, hudfix, tags)
- `tools/thugkit/` — the Go build/apply core (own repo)
- `mods/` — the mod source-of-truth (own repo)
- `gui/` — optional web-UI installer (pure Go, wraps the CLI — zero native deps)

> Revert never ships THUG2 game files, no-CD executables, or licensed/derivative
> packs. You must own the game; some optional content (HQ A/V, brand decks) is
> user-supplied. See [docs/INSTALL.md](docs/INSTALL.md).

## License
Revert is MIT licensed (see [LICENSE](LICENSE)). It redistributes a few third-party
components under their own licenses, including a **modified** build of DXVK (zlib) for the
macOS lane, and ThirteenAG's WidescreenFixesPack and Ultimate ASI Loader (MIT). They are
credited in [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).
