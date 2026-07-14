# GlyphFix on macOS — the RE brief

## The one-line problem
`VV.GlyphFix.asi` makes THUG2 **freeze at the main menu** under Wine on this M1. It is the only
one of the three VV .asi mods that does. Ship-blocking for the glyphs feature on macOS; the lane
ships with GlyphFix OFF and this is what you're trying to change.

## What GlyphFix is for (the feature)
THUG2's trick-combo prompts (e.g. "\b4 + \b2") are font glyph tokens. The PC font renderer draws
the four **FACE-button** slots (0-3) via the keyboard-key path — so they show "kp2" etc. instead
of a button glyph. Slots 4-7 (d-pad) already draw as glyphs. GlyphFix flips the face buttons to
the glyph path so trick prompts show Xbox/PS/GC button icons. Cosmetic, but it's the polish.

## How it does it (`~/thug2/tools/glyphfix/glyphfix.cpp`) — three distinct actions
1. **Branch NOP (the renderer lever).** NOPs `cmp al,4 / jb <keyname>` (bytes `72 0c` → `90 90`)
   at **`0x4ced6f`** and **`0x4cff38`**. This is what routes face buttons to the glyph path.
2. **Font imm patch.** Rewrites the buttons-font name immediate at **`0x48d983`** (default
   "ButtonsXbox") to "ButtonsPs2"/"ButtonsNgc" — ONLY for PS/GC styles. **In the default xbox
   style this is a no-op**, so it is NOT involved in the default-config freeze.
3. **Live flag poll.** A worker thread sleeps 6s, applies the branch NOP, then every 1s reads the
   in-game "Button Glyphs" menu flags by pointer-chasing raw addresses
   (`flagmgr = *(*0x7ce478 + 0x20)`, bits at `+0x5f0`) to toggle the renderer live.

## ✅ RULED OUT — do not spend time re-testing these (all falsified by verified play-tests)
The-core tried FOUR fixes; each one still froze, proven with an md5-checked build the user played:
1. **W^X / page protection.** `VirtualProtect(PAGE_EXECUTE_READWRITE)` (which macOS forbids) →
   changed to `PAGE_READWRITE` + `FlushInstructionCache`. Still froze.
2. **Poll fault-probe.** The poll used `IsBadReadPtr` (validates by faulting-and-catching; on
   Apple Silicon each fault is a Mach round-trip through Rosetta). Replaced with `VirtualQuery`
   (no fault). Still froze.
3. **Killed the live poll** entirely (patch once at 6s, no worker loop). Still froze.
4. **Patched COLD in DllMain**, before the render thread exists (no concurrent execution of the
   page). Still froze.

**Conclusion: it is NOT the patch mechanism, the poll, or the timing.** Every build that applies
the two branch NOPs freezes, however and whenever the NOP is written. The wrong exe is also ruled
out — `game-playable-us/THUG2.exe` md5 is `d464781a2863c833c640f7ff6d377ffe`, the exact binary the
mod works against on Linux/Windows.

## 🎯 THE ACTUAL HYPOTHESIS (where to dig)
**Making the face buttons take the GLYPH render path is what hangs — the enabled path itself, not
the act of enabling it.** On Windows/Linux that path draws fine; on Wine-mac it wedges. Leading
theory: **the "ButtonsXbox" glyph font for slots 0-3 isn't actually available/loaded on Wine-mac**,
and the "kp2" keyboard path was masking a font-load or glyph-lookup code path that loops or blocks
when it's the one taken. (The d-pad glyphs 4-7 already work, so a *different* glyph set for 0-3, or
a per-slot lookup, may be the difference.)

## Concrete next experiments (in rough priority)
1. **Split the two branches.** NOP ONLY `0x4ced6f`, then ONLY `0x4cff38` (two separate builds).
   If one freezes and the other doesn't, you've halved the problem and identified which render call
   is bad. (Add a build-time `#define` or an env gate — see the instrumentation note.)
2. **Trace the font render at the freeze.** Launch with `WINEDEBUG=+relay` narrowed to the font
   module, or `+font`, and capture what call is in flight when the menu wedges. The lane's normal
   launch env sets `err-all`/`fixme-all` which SUPPRESSES traces — run the game DIRECTLY with your
   own WINEDEBUG (see the direct-launch recipe below), not via `revert run`.
3. **Is the glyph font present?** Check whether "ButtonsXbox" (and the d-pad glyphs that DO work)
   live in the same fonts.prx and whether Wine-mac loads it. If face-button glyphs are simply
   absent, the fix might be shipping/repointing the font rather than touching code at all.
4. **Watch for a hang vs a loop.** Attach/observe: is the main thread spinning (100% one core,
   infinite loop in the glyph path) or blocked (0%, waiting on something)? `sample THUG2.exe` /
   Activity Monitor "Sample Process" on the wedged process will give a native+Rosetta stack —
   that could point straight at the offending function.

## Build / deploy / test loop (LOCAL — you're on the Mac)
- **Build:** `cd ~/thug2/tools/glyphfix && i686-w64-mingw32-g++ -O2 -shared -static -o
  VV.GlyphFix.asi glyphfix.cpp -luser32 -lkernel32`
- **Select + build the edition:** set `~/thug2/revert.conf.local` → `MAC_VV_ASI="glyphfix"`, then
  `cd ~/thug2 && ./revert build --fast`.
- **⚠️ Verify the bytes landed:** `md5 ~/thug2/tools/glyphfix/VV.GlyphFix.asi` MUST equal
  `md5 ~/thug2/game-playable-us/scripts/VV.GlyphFix.asi` after the build. (This is about "did MY
  new build reach the game", not matching the-core — a different mingw version gives different
  bytes than the committed pristine .asi, which is fine.)
- **Play it:** `cd ~/thug2 && ./revert run qol`, or open the app. WATCH THE SCREEN — a frozen game
  still shows ~130% CPU, so liveness is not a valid test (this burned the-core three times).
- **Direct launch with your own WINEDEBUG (for traces):**
  ```
  cd ~/thug2/game-playable-us
  export WINEPREFIX=~/.wine-thug2-ws
  export WINEDLLOVERRIDES="mscoree,mshtml=;d3d9=n,b;winmm=n,b;dinput8=n,b"
  export MVK_CONFIG_LOG_LEVEL=1 DXVK_CONFIG_FILE=~/thug2/tools/dxvk-mac/dxvk.conf
  export WINEDEBUG="+font"     # or +relay, +seh, etc.
  "$HOME/thug2/.revert-cache/mac/wine/Wine Stable.app/Contents/Resources/wine/bin/wine" \
      explorer /desktop=thug2,1440x900 THUG2.exe > ~/glyph-trace.log 2>&1
  ```
  (This skips vv-run.bat/the pad bridge, which is fine for diagnosing the freeze.)
- **Kill a stuck game:** `killall -9 THUG2.exe` (NEVER `pkill -f THUG2.exe` — it matches your own
  shell's command line and kills your session).

## Instrumentation tip (worked well for the-core)
Add env-gated logging + test modes to glyphfix.cpp so ONE build can bisect itself:
`VV_GLYPH_LOG=1` → append timestamped lines to a log (fopen/fclose EACH line — the process may be
dying, so flush every write); `VV_GLYPH_TEST=<mode>` → skip the thread / skip the patch / skip the
flags. This isolated "which action" cheaply. Strip it before handing back the fix.

## When you've got it
Write `RESULTS.md` (root cause + the exact glyphfix.cpp change + the confirmed build), leave the
working `glyphfix.cpp` in place, and stop. The-core does the persona commit and the Linux/Windows
regression check. **Do not commit or push from this Mac** (its git identity is the real name).
