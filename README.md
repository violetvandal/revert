# Revert — THUG2: Violet Vandal Edition toolkit

**Revert** builds and runs the *definitive, modernized Tony Hawk's Underground 2* on
Linux / Steam Deck (Windows via the cross-platform core). It ships **tooling, never
game data** — you bring your own THUG2 copy.

One front door, three lanes:

| Lane | What |
|------|------|
| **Vanilla** | clean THUG2 + no-CD + widescreen + controller |
| **QOL-Modded** *(flagship)* | the curated mod suite (MOD OPTIONS / LEVEL MODS, HUD fix, HQ A/V, custom tags…) — every mod default-off, vanilla one click away |
| **Online** | THUG Pro (bundled optional companion, isolated prefix) |

## Quick start
```sh
./revert doctor                         # check prerequisites
./revert setup                          # one-time Wine/DXVK/controller + prefixes
./revert acquire-game-data --folder /path/to/your/THUG2   # your copy -> pristine base
./revert build                          # build the edition (reproducible)
./revert run qol                        # play  (also: vanilla | online)
```

`revert run qol --soundtrack radio` plays the royalty-free "Violet Vandal Radio"
soundtrack (stream-safe). `revert tag <image>` imports a custom Create-A-Graphic tag.
`revert run qol --glyphs playstation` themes the on-screen trick-combo button prompts
(Xbox / PlayStation / GameCube / keyboard; `auto` picks Xbox on Steam Deck). You can also
change it in-game under **Game Options → MOD OPTIONS → Button Glyphs**.

See [docs/INSTALL.md](docs/INSTALL.md) for the full setup,
[docs/STEAMDECK.md](docs/STEAMDECK.md) for the Steam Deck lane,
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how it's built.

## What's in the box
- `revert` — the dispatcher (this is the only command you run)
- `revert.conf` — single source of truth (paths, wine runtime, lanes)
- `share/` — bash planes (`run/`, `setup/`) + shippable assets (controller, hudfix, tags)
- `tools/thugkit/` — the Go build/apply core (own repo)
- `mods/` — the mod source-of-truth (own repo)

> Revert never ships THUG2 game files, no-CD executables, or licensed/derivative
> packs. You must own the game; some optional content (HQ A/V, brand decks) is
> user-supplied. See [docs/INSTALL.md](docs/INSTALL.md).
