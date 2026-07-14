# THUG2 macOS lane — patched DXVK d3d9 + working recipe (Apple Silicon / M1)

> **This is now packaged.** `revert setup` on a Mac does everything below automatically
> (installs Wine, creates the prefix, drops this d3d9.dll in, imports the controller
> bindings and probes the pad GUID, builds the trick bridge, writes the .app bundles), and
> `revert run qol` launches it. The manual recipe is kept here as the reference for what
> the code does and why. See `internal/core/mac.go`.
>
> The DXVK config and shader cache are passed by ENVIRONMENT (`DXVK_CONFIG_FILE` /
> `DXVK_STATE_CACHE_PATH`) rather than copied next to THUG2.exe, so the warmed cache
> survives a `revert build` (which re-lays the edition directory) instead of being wiped
> with it.

Runs THUG2 (32-bit D3D9, 2004) **GPU-accelerated** on Apple Silicon under free WineHQ
`wine-stable` (11.0) + MoltenVK, via a DXVK we patched to survive the lack of geometry
shaders on Metal. Proven end-to-end on an M1 (macOS 26.5): boot, GPU d3d9, audio,
widescreen, keyboard, controller (menus + gameplay), trick glyphs, HUD fix, controller
text entry, **stable menu**.

## Files
- `d3d9-dxvk-patched-m1.dll` — 32-bit DXVK 1.10.3 (Gcenx/DXVK-macOS `1.10.x`) patched to run
  Direct3D 9 on Apple Silicon / MoltenVK. Copy into the wine prefix as
  `drive_c/windows/syswow64/d3d9.dll`.
- `dxvk-apple-silicon-d3d9.patch` — the source patches (6 total; also in memory
  `project_macos_lane`). Rebuild: clone Gcenx/DXVK-macOS (1.10.x), `git apply`, then
  `meson setup --cross-file build-win32.txt --buildtype release -Denable_d3d11=false
  -Denable_d3d10=false -Denable_dxgi=false build32 && ninja -C build32 src/d3d9/d3d9.dll`.
  Needs: i686-w64-mingw32, meson, ninja, glslang.
- `dxvk.conf` — the shipping DXVK config (frame pacing + shader cache). Addressed via
  `DXVK_CONFIG_FILE`, not copied next to `THUG2.exe`.
- `harness/` — the Mac-side iteration scripts (launcher body, kill, launchctl-launch).

## ⚠️ dxvk.conf does NOT fix the menu freeze (corrected 2026-07-12)
This README used to claim `enableAsync = True` was **required** for menu stability, and that
synchronous shader compilation on the render thread was the intermittent "menu freezes a few
seconds in" hang. **That diagnosis was wrong**, and it sent a later session hunting the glyph
render path for a bug that was never there.

The freeze was `VV.HudFix` and `VV.GlyphFix` **rewriting live game code from a worker thread**
(a torn write into a hot function, an illegal `PAGE_EXECUTE_READWRITE` under macOS W^X, and no
instruction-cache flush). It is unsound under Rosetta, and it is fixed in the mods themselves,
which now patch cold in `DllMain`. `PERF.md` measured `enableAsync` and `numCompilerThreads`
directly and found **both non-causal**.

`enableAsync` stays on its own merits (it should shorten first-seen-shader hitches), but it is
not a freeze fix. If the menu ever hangs again, look at what is patching game code.

## Working recipe (M1)
1. Engine: WineHQ `wine-stable` (`brew install --cask wine-stable`, then strip quarantine:
   `sudo xattr -dr com.apple.quarantine "/Applications/Wine Stable.app"`).
2. Fresh prefix (e.g. `~/.wine-thug2-ws`), `wineboot -i` with `WINEDLLOVERRIDES='mscoree,mshtml='`.
3. Copy `d3d9-dxvk-patched-m1.dll` → `<prefix>/drive_c/windows/syswow64/d3d9.dll`.
4. Game dir (rsynced `game-playable-us`): drop in `dxvk.conf`; keep `winmm.dll` (Ultimate ASI
   Loader) + `scripts/*.asi` (WidescreenFix + VV.GlyphFix/HudFix/KeyboardGrid — all three are
   safe since they were changed to patch cold in `DllMain`; this has nothing to do with async).
   Set the res in `TonyHawksUnderground2.WidescreenFix.ini` (16:10 e.g. 1440x900 to match the
   Mac panel, no stretch).
5. Launch (via a `.app` bundle + `launchctl asuser`, NOT `open`):
   `WINEDLLOVERRIDES="mscoree,mshtml=;d3d9=n,b;winmm=n,b" MVK_CONFIG_LOG_LEVEL=1
   wine explorer /desktop=thug2,1440x900 THUG2.exe`
   The virtual desktop is mandatory (THUG2 asks for a 640x480 fullscreen mode-change the Mac
   can't do). `MVK_CONFIG_LOG_LEVEL=1` silences MoltenVK's per-draw primitive-restart warning.
   See `harness/` for the exact scripts.
6. Controller: bind once via THUG2's `Launcher.exe` ("360 controller" option) → Save.

See memory `project_macos_lane` for the full RE story.
