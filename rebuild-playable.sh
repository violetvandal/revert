#!/usr/bin/env bash
#
# rebuild-playable.sh — reconstruct game-playable-us entirely from sources.
#
# This is the reinstallability backbone: the playable install is DERIVED, never precious.
# Everything here comes from a tracked source, so the modded+patched game can always be
# rebuilt (and `git`-saved mods stay the single source of truth).
#
#   layers, in order:
#     1. clean base data        <- game-pristine-us/Data            (untouched master)
#     2. no-CD executable        <- game-modded-vanilla/THUG2.exe    (runtime, private)
#     3. widescreen (WSFix)      <- mods/apply-widescreen.sh
#     4. data mods               <- thugkit (Go applier, tools/thugkit) — source .ns + HQ-texture pack
#                                   (legacy bash path mods/apply-mods.sh kept as reference/fallback)
#     5. HQ audio/video patch    <- mods/apply-hq-audio.sh  (your supplied pack)
#
#   --fast   skip the HQ audio re-apply (~866 MB) — use during mod iteration
#            (audio rarely changes; the day-to-day loop is just apply-mods --only <mod>)
#
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PRISTINE="$ROOT/game-pristine-us"
DEST="$ROOT/game-playable-us"
RUNTIME_EXE="$ROOT/game-modded-vanilla/THUG2.exe"          # no-CD exe source
WSFIX_ZIP="$ROOT/tools/TonyHawksUnderground2.WidescreenFix.zip"
HQ_AUDIO_PACK="$ROOT/tools/THUG2_HQ_Xbox.7z"

FAST=0; [[ "${1:-}" == "--fast" ]] && FAST=1
log(){ printf '\033[1;34m[rebuild]\033[0m %s\n' "$*"; }
err(){ printf '\033[1;31m[rebuild:error]\033[0m %s\n' "$*" >&2; exit 1; }

[[ -d "$PRISTINE/Data/pre" ]] || err "pristine base missing: $PRISTINE"
[[ -f "$RUNTIME_EXE" ]]       || err "no-CD exe missing: $RUNTIME_EXE"

# thugkit — the canonical Go mod applier (replaces mods/apply-mods.sh). One static
# binary: compiles NeverScript in-process + packs .prx, no python/ns deps. Built
# from source here; the shipped installer (Revert) will carry a prebuilt binary.
THUGKIT="$ROOT/tools/thugkit/thugkit"
apply_mods() {  # $1 = install dir
  "$THUGKIT" apply "$1" --mods "$ROOT/mods"
}

# VV.HudFix — a small custom .asi (runs alongside ThirteenAG's WidescreenFix) that pulls
# the score cluster + goal-points readout to the true top-left on widescreen, undoing
# FixHUD's pillarbox-centering for just those elements. Resolution-independent (reads the
# live offset at runtime). Built from source via mingw if available, else uses the committed
# prebuilt; copied into scripts/ next to the WidescreenFix .asi. See tools/hudfix/hudfix.cpp.
install_hudfix() {  # $1 = install dir
  local dest="$1/scripts"
  [[ -d "$dest" ]] || return 0
  local src="$ROOT/tools/hudfix/hudfix.cpp" asi="$ROOT/tools/hudfix/VV.HudFix.asi"
  if command -v i686-w64-mingw32-g++ >/dev/null && [[ -f "$src" ]]; then
    i686-w64-mingw32-g++ -O2 -shared -static -static-libgcc -static-libstdc++ -masm=att \
      -o "$asi" "$src" -lkernel32 2>/dev/null && log "  built VV.HudFix.asi" \
      || log "  (VV.HudFix build failed; using prebuilt if present)"
  fi
  [[ -f "$asi" ]] && cp -f "$asi" "$dest/VV.HudFix.asi" && log "  installed VV.HudFix.asi -> scripts/"
}

# Trick-combo button glyphs: THUG2 PC renders face-button glyph tokens (\b0..\b3) as the bound
# keyboard key ("kp2"); this .asi flips the renderer's face-button branch so they draw as the
# ButtonsXbox controller glyphs (Edit Tricks / Create-A-Trick). See tools/glyphfix/glyphfix.cpp.
install_glyphfix() {  # $1 = install dir
  local dest="$1/scripts"
  [[ -d "$dest" ]] || return 0
  local src="$ROOT/tools/glyphfix/glyphfix.cpp" asi="$ROOT/tools/glyphfix/VV.GlyphFix.asi"
  if command -v i686-w64-mingw32-g++ >/dev/null && [[ -f "$src" ]]; then
    i686-w64-mingw32-g++ -O2 -shared -static -static-libgcc -static-libstdc++ -masm=att \
      -o "$asi" "$src" -lkernel32 2>/dev/null && log "  built VV.GlyphFix.asi" \
      || log "  (VV.GlyphFix build failed; using prebuilt if present)"
  fi
  [[ -f "$asi" ]] && cp -f "$asi" "$dest/VV.GlyphFix.asi" && log "  installed VV.GlyphFix.asi -> scripts/"
}

# CAS asset mods that live OUTSIDE the .ns/.qb mod pipeline (texture recolours in the
# skater model archives). Re-applied here so a rebuild keeps them. See the script's
# header for why the panty is the white region of bb850270 in Skater_F_pvlegs.
apply_cas_asset_mods() {  # $1 = install dir
  command -v python3 >/dev/null || { log "  (python3 absent — skipping CAS asset mods)"; return 0; }
  python3 "$ROOT/tools/save/apply_panty_color.py" "$1" 130 50 190
  # decks-pack: script half is an ns-inject mod; this injects its deck textures
  # into skaterparts.prx (see tools/save/apply_deck_pack.py for why it's separate).
  # licensed art payloads live in gitignored blob/ (sources tracked, art not published)
  [[ -d "$ROOT/mods/src/decks-pack/blob/textures" ]] && \
    python3 "$ROOT/tools/save/apply_deck_pack.py" "$1" "$ROOT/mods/src/decks-pack/blob/textures"
  # playas-pro: script half is ns-inject; this injects the guest/pro skater models
  [[ -d "$ROOT/mods/src/playas-pro/blob/models" ]] && \
    python3 "$ROOT/tools/save/apply_playas_models.py" "$1" "$ROOT/mods/src/playas-pro/blob/models"
  # custom wall-slap stickers: auto-import every image in tools/save/stickers/
  # into the first Graphics CAGR slots (loose + cagpieces.prx). See apply_stickers.py.
  [[ -d "$ROOT/tools/save/stickers" ]] && \
    python3 "$ROOT/tools/save/apply_stickers.py" "$1"
  # custom Create-A-Graphic tags: re-install every .GRF/image in tools/save/tags/
  # (the persona tag's .GRF + any custom-image tags' cagpieces.prx sprite). This is
  # outside the .ns pipeline and the Save/ tag would be lost on a fresh build, so it
  # lives in source and is re-applied here. See apply_tags.py.
  [[ -d "$ROOT/tools/save/tags" ]] && \
    python3 "$ROOT/tools/save/apply_tags.py" "$1"
  # Violet Vandal Radio is a LAUNCH-TIME lane, not a build step: run-playable-ge.sh
  # (Original) / run-playable-radio-ge.sh (radio) call tools/bink/radio/set_soundtrack.sh
  # before launch. Base build keeps the Original (licensed) soundtrack.
  :
}
command -v go >/dev/null || err "go toolchain required to build thugkit (the mod applier)"
log "build thugkit (Go mod applier)"
( cd "$ROOT/tools/thugkit" && go build -o thugkit ./cmd/thugkit ) || err "thugkit build failed"

mkdir -p "$DEST" "$DEST/Save"
reset_from_pristine() {  # $1 = subpath under Data (e.g. "" for all, "pre" for mods only)
  local sub="$1"
  if command -v rsync >/dev/null; then
    rsync -a --delete "$PRISTINE/Data/$sub/" "$DEST/Data/$sub/"
  else
    rm -rf "$DEST/Data/$sub"; cp -a "$PRISTINE/Data/$sub" "$DEST/Data/$sub"
  fi
}

if (( FAST )); then
  # Iteration mode: only reset the mod-affected Data/pre (ALL mods live here), re-apply
  # mods. Leaves streams/movies (HQ audio/video) and the exe/widescreen runtime untouched.
  log "1/2  reset Data/pre <- pristine (audio/video & runtime kept)"
  reset_from_pristine pre
  log "2/2  data mods (source .ns + HQ-texture pack) — via thugkit"
  apply_mods "$DEST"
  log "      CAS asset mods (panty colour)"
  apply_cas_asset_mods "$DEST"
  log "      HUD fix (top-left score on widescreen)"
  install_hudfix "$DEST"
  log "      controller glyphs for trick combos"
  install_glyphfix "$DEST"
  log "Done (--fast). Play: $ROOT/run-playable-ge.sh"
  exit 0
fi

log "1/5  base data <- pristine (preserving Save/)"
reset_from_pristine ""
for f in binkw32.dll gdiplus.dll THUG2.ico Launcher.exe Launcher_fr.exe Launcher_gr.exe; do
  [[ -f "$PRISTINE/$f" ]] && cp -f "$PRISTINE/$f" "$DEST/"
done
cp -f "$PRISTINE"/*.url "$DEST/" 2>/dev/null || true

log "2/5  no-CD executable"
cp -f "$RUNTIME_EXE" "$DEST/THUG2.exe"

log "3/5  widescreen (WSFix winmm loader + scripts)"
[[ -f "$WSFIX_ZIP" ]] && "$ROOT/mods/apply-widescreen.sh" "$DEST" "$WSFIX_ZIP" || err "WSFix zip missing: $WSFIX_ZIP"

log "4/5  data mods (source .ns + HQ-texture pack) — via thugkit"
apply_mods "$DEST"
log "     CAS asset mods (panty colour)"
apply_cas_asset_mods "$DEST"
log "     HUD fix (top-left score on widescreen)"
install_hudfix "$DEST"
log "     controller glyphs for trick combos"
install_glyphfix "$DEST"

if [[ -f "$HQ_AUDIO_PACK" ]]; then
  log "5/5  HQ audio/video patch"
  "$ROOT/mods/apply-hq-audio.sh" "$DEST" "$HQ_AUDIO_PACK"
else
  log "5/5  HQ audio pack not found ($HQ_AUDIO_PACK) — skipping"
fi

log "Done. Play: $ROOT/run-playable-ge.sh"
