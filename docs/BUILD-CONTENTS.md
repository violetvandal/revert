# What's in `game-playable-us/` — every change from pristine

`game-playable-us/` is the **QOL-Modded edition** — the output of `./revert build` (the
`EDITION_QOL` lane). It's a full copy of the clean base (`game-pristine-us/`) with every
mod and A/V layer composited on top. It is **gitignored and disposable**: `./revert build`
reconstructs it identically from tracked sources + your supplied packs, so nothing here is
precious.

Measured against `game-pristine-us/`: **22 files added · 20 changed · 13 removed.** Every
one is accounted for below.

---

## 1. Runtime / executable layer
The exe + loader changes that make it run on modern Wine/PC in widescreen with a controller.

| Piece | Change | What it is |
|---|---|---|
| `THUG2.exe` | 3.93 MB → **2.70 MB** | **No-CD** executable (SafeDisc disc check stripped — the disc exe won't run under Wine) |
| `winmm.dll` | **added** (2.16 MB) | ThirteenAG **WidescreenFix** ASI loader, installed *as* `winmm.dll` (deliberately **not** `dinput8.dll`) so the native controller still enumerates |
| `scripts/TonyHawksUnderground2.WidescreenFix.asi` + `.ini` | **added** | the widescreen / FOV / HUD-resolution fix itself |
| `scripts/VV.HudFix.asi` | **added** (92 KB) | our custom mod: moves the score + goal-points HUD to true top-left on widescreen |
| `scripts/VV.GlyphFix.asi` | **added** (96 KB) | our custom mod: shows Xbox / PlayStation / GameCube / keyboard button glyphs for trick combos |

## 2. Gameplay script mods (`Data/pre/*.prx`)
Recompiled NeverScript injected into your own base archives (the open QOL mod set).

| Archive | Change | Mod(s) |
|---|---|---|
| `qb_scripts.prx` | 1.367 → **1.382 MB** (under the ~1.43 MB boot ceiling) | front-end framework: **MOD OPTIONS** menu, **LEVEL MODS**, per-level **"Skip Goal"**, **Silence Phone** + the deck/play-as-pro script halves |
| `mainmenu_scripts.prx` | 821 → 836 KB | adds the **MOD OPTIONS** entry to the main menu |
| `AU_scripts.prx` (+ minor `AuTempProfile.prx`) | grew | Australia — **seagull SFX** + level script |
| `BE_scripts.prx` | grew | Berlin — **keep area-music** in music zones |
| `NO_scripts.prx` | grew | New Orleans — keep-soundtrack + **balcony respawn** + **disable-traffic** |
| `ST_scripts.prx` | grew | Skatopia — **car/traffic toggle** |
| `TR_scripts.prx` | grew | Training level script |
| `BO_scripts.prx` | grew | Boston — **disable-traffic toggle** |

## 3. Create-A-Skater content (`Data/pre/` — private/licensed extras)
Backported from THUG Pro; these are the licensed/derivative packs kept out of the public repos.

| Archive | Change | What |
|---|---|---|
| `skaterparts.prx` | 5.6 → **10.4 MB** | **HD deck graphics** in Create-A-Skater (`decks-pack`) |
| `skaterparts_temp.prx` | 3.98 → **10.3 MB** | **Play-as-Pro** guest/pro skater models (`playas-pro`) + the purple **panty-colour** recolour |
| `casfiles.prx` | 77 → 80 KB | guest skater `.cas` definitions (play-as-pro) |

## 4. HQ level textures (`Data/pre/` — optional / bring-your-own)
Community HQ textures for the classic levels (`hq-level-textures`, `optional=true`).

| Archive | Change |
|---|---|
| `CAscn.prx` | 2.31 → **4.49 MB** (Canada) |
| `DJscn.prx` | 1.01 → **3.56 MB** (Downhill Jam) |
| `SCscn.prx` | 1.27 → **4.22 MB** (School) |

## 5. Custom stickers + tags
| Piece | Change | What |
|---|---|---|
| `Data/images/CAGR/Graphics/grap_1..3.img.xbx` (+ `.orig` backups) | changed / added | custom Create-A-Graphic **"sticker slap"** art (customs show at the top of the Graphics list) |
| `cagpieces.prx` | +3 KB | the in-editor **thumbnails** for those stickers |
| `Save/*.GRF` | **added** | custom in-game **tags** — `VioletVandal.GRF` (the persona tag, seeded reproducibly) + your in-game test tags |
| `Save/*.SKA` | **added** | your **created-skater saves** (Custom Skater / Custom Skater 2) |

## 6. HQ audio + video (`Data/streams`, `Data/movies`)
| Piece | Change | What |
|---|---|---|
| `Data/streams/music/*.bik` | 456 → **599 MB** | **HQ Xbox music** overlaid on the licensed soundtrack (marker `8541624c.bik` > 8 MB = applied). The PC `pcm.*` (spoken dialog) is deliberately **kept** so voices aren't silenced |
| `Data/movies/` (`Credits.bik`, `demo_1/2.bik`) | swapped | **HQ Xbox cutscene/attract video**; the PC boot-logo reel (`intro`, `ATVI`, `NSlogo`, `Beenox_Shift`, `Dolby_Digital`, `Intel_intro`) is not carried into the build |
| `Data/movies/bik/vvcredits.bik` + `vvabout.bik` | **added** | our custom in-game **Skatepark Project credits movie** + donation card |
| `Data/streams/music_original/` + `Data/streams/.soundtrack` | **added** | the soundtrack system's **HQ-aware snapshot** + mode marker (v1.1.1) — lets the launcher swap Original ↔ "Violet Vandal Radio" without losing HQ audio |

## 7. Removed from pristine (cruft / replaced)
- `Activision*.url` — dead marketing shortcuts.
- PC boot-logo movies — replaced by the HQ movie set (see §6).
- *(Note: `PRISTINE_README.txt` and `SHA256SUMS.txt` exist only in `game-pristine-us/` — they're the verification metadata for the clean rip, not game files, so they're correctly absent here.)*

---

## How it's all rebuilt
```sh
./revert build          # full: base copy + no-CD + widescreen + mods + CAS + tags + HQ A/V
./revert build --fast   # quick: reset Data/pre + re-apply script mods only (SKIPS HQ A/V)
```
Everything above is derived from `game-pristine-us/` + the tracked mod sources + your
supplied packs (no-CD exe, WidescreenFix zip, HQ Xbox pack). Re-running produces the same
edition. **`--fast` skips the HQ A/V overlay** — so if the HQ music ever goes missing, the
usual cause is a `--fast` build; a full `./revert build` (or `./revert acquire-hq audio`
then `./revert build`) brings it back.
