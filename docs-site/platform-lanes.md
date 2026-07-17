# Platform lanes

The edition ships **one build, three ways to play**: Vanilla (preserved), QOL-Modded
(flagship), and Online (a THUG Pro companion). A "lane" is a launch configuration, defined
entirely in `revert.conf` and executed by `share/run/revert-run.sh`. This page covers how a
lane is defined, how launch works per platform, and how to add one.

## How a lane is defined

Each lane is a set of variables in `revert.conf`:

```
LANE_<NAME>_DIR         the game directory to launch from
LANE_<NAME>_PREFIX      the Wine prefix (isolated per lane where needed)
LANE_<NAME>_EXE         the executable to run
LANE_<NAME>_ENV         extra environment (e.g. DXVK / Proton knobs)
LANE_<NAME>_HOOKS       pre-launch hooks (soundtrack swap, trigger bridge)
LANE_<NAME>_SOUNDTRACK  which soundtrack to bake/swap for this lane
```

`EDITION_*` variables describe the edition itself (names, paths, the app-menu entry). Because
both planes read `revert.conf`, the launch config and the build config never drift.

## Launch on Linux and the Deck (`revert run <lane>`)

`share/run/revert-run.sh` launches under GE-Proton using the selected lane's variables. Two
things the engine cannot do itself are handled as **hooks** before launch:

- **Soundtrack swap.** The engine cannot toggle the soundtrack live, so the hook swaps the
  relevant `.prx` entry (shelling to `thugkit prx`) for the lane's chosen soundtrack.
- **Trigger bridge.** An evdev bridge maps the L2/R2 shoulder combos that the game cannot
  bind natively. It matches the pad by **capability** (ABS_Z + ABS_RZ + BTN_TL + BTN_TR), not
  by name, so any XInput pad works.

The runner also resolves the button-glyph style (`GLYPH_STYLE` / `--glyphs`, with Steam Deck
auto-detecting Xbox) into `$VV_GLYPHS`, which the glyph `.asi` reads. Run options:
`--soundtrack original|radio` and `--glyphs xbox|playstation|gamecube|keyboard`.

!!! note "Never run Wine against a prefix mid-game"
    On the Deck, play in Gaming Mode and never run a `wine` command against the prefix while
    the game is running; it disturbs the pad binding. See the player Steam Deck guide for the
    controller specifics.

## Launch on Windows (native)

On Windows there is no Wine. The Go front door's `internal/core` runs the commands natively,
and `run` simply launches `THUG2.exe` from the edition directory (so the `winmm.dll` ASI
loader and the `.asi`s resolve), with `$VV_GLYPHS` set and the soundtrack swap ported to Go
(shelling to `thugkit prx`). The `vv-padbridge` helper supplies the L2/R2 combos via native
syscalls. Crucially, **on Linux the same core commands delegate to the bash dispatcher**, so
bash stays authoritative and the two platforms cannot diverge. See
[Architecture](architecture.md#the-windows-lane-native-no-wine).

## Adding a lane

1. Define `LANE_<NAME>_{DIR,PREFIX,EXE,ENV,HOOKS,SOUNDTRACK}` in `revert.conf`.
2. If it needs a distinct build (a different soundtrack baked in, say), wire it through the
   [build pipeline](build-pipeline.md) options; otherwise it reuses `game-playable-us`.
3. Add any pre-launch hook it needs (soundtrack, input) following the existing hook pattern in
   `share/run/revert-run.sh`.
4. If Windows should support it, mirror the launch in `internal/core` (remember Linux delegates
   to bash, so implement the native path and let the delegate cover Linux).
5. Test it end to end on the real platform: install → build → `run <name>` → plays with a
   controller. A lane is not done until that full path is captured.
