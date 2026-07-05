#!/usr/bin/env bash
# set_soundtrack.sh <gamedir> <radio|original>
#
# Launch-time soundtrack switch for THUG2 (a true in-game runtime toggle is
# engine-impossible: LoadPermSongs binds the jukebox once at cold boot and
# GlobalFlags don't persist — see memory project_streaming_mode). This swaps
# the actual stream files AND the jukebox titles BEFORE the game starts, so the
# choice takes effect at the moment the engine binds it.
#
#   radio    -> overwrite jukebox .bik with royalty-free CC-BY tracks + radio titles
#   original -> restore THIS BUILD's original soundtrack + original titles
#
# "original" means the soundtrack the BUILD produced — which is HQ if HQ audio was
# applied — NOT the pristine PC rip. The built original is snapshotted to
# music_original/ the first time we switch to radio, and restored from there.
# When no snapshot exists the current music already IS the built original, so we
# leave the .bik untouched (this is what stopped HQ audio from being clobbered by
# pristine PC audio on every launch). Idempotent via a .soundtrack marker.
#
# Pure file ops + prx entry swap (no wine).
set -euo pipefail
GD="${1:?usage: set_soundtrack.sh <gamedir> <radio|original>}"
MODE="${2:?usage: set_soundtrack.sh <gamedir> <radio|original>}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
MUSIC="$GD/Data/streams/music"
ORIG="$GD/Data/streams/music_original"        # snapshot of this build's original (HQ-aware) soundtrack
STATE="$GD/Data/streams/.soundtrack"          # current mode marker (idempotency)
PRX="$GD/Data/pre/qb_scripts.prx"
VAR="$ROOT/tools/bink/radio/variants"
ENTRY='scripts/game/skater/skater_sfx.qb'     # prx.py find() maps / -> \

swap_titles() {  # $1 = variant .qb (jukebox title table — a rebuildable, gitignored artifact)
  if [ ! -f "$1" ]; then
    # Not shipped in slim/public clones. The build already carries the matching titles,
    # so skipping is harmless — never let a missing title table block launch.
    echo "  (jukebox title table $(basename "$1") not present — keeping the build's titles)"
    return 0
  fi
  python3 "$ROOT/tools/prx/prx.py" replacez "$PRX" "$ENTRY" "$1" "$PRX" >/dev/null
}

current="$(cat "$STATE" 2>/dev/null || echo unknown)"
if [ "$current" = "$MODE" ]; then
  echo "soundtrack already $MODE — no change"
  exit 0
fi

case "$MODE" in
  radio)
    [ -d "$ROOT/tools/bink/radio/bik" ] || { echo "no radio .bik (run encode_bik.sh)"; exit 1; }
    # Preserve the built original soundtrack (HQ or not) so we can restore it later.
    if [ ! -d "$ORIG" ] || [ -z "$(ls -A "$ORIG" 2>/dev/null)" ]; then
      mkdir -p "$ORIG"; cp "$MUSIC"/*.bik "$ORIG"/
    fi
    python3 "$ROOT/tools/bink/radio/apply_radio.py" "$GD" >/dev/null
    swap_titles "$VAR/skater_sfx_radio.qb"
    echo "soundtrack -> Violet Vandal Radio (royalty-free)"
    ;;
  original)
    if [ -d "$ORIG" ] && [ -n "$(ls -A "$ORIG" 2>/dev/null)" ]; then
      cp "$ORIG"/*.bik "$MUSIC"/        # returning from radio -> restore the built (HQ) original
    fi
    # else: no snapshot -> music/ already IS the built original (e.g. a fresh HQ build);
    #       leave the .bik untouched so HQ audio is never overwritten with pristine PC audio.
    rm -f "$MUSIC/VIOLET_VANDAL_RADIO_credits.txt"
    swap_titles "$VAR/skater_sfx_original.qb"
    echo "soundtrack -> Original"
    ;;
  *) echo "usage: set_soundtrack.sh <gamedir> <radio|original>"; exit 2 ;;
esac
echo "$MODE" > "$STATE"
