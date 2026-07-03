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
#   original -> restore licensed .bik from pristine + original titles
#
# Pure file ops + prx entry swap (no wine). Idempotent.
set -euo pipefail
GD="${1:?usage: set_soundtrack.sh <gamedir> <radio|original>}"
MODE="${2:?usage: set_soundtrack.sh <gamedir> <radio|original>}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
MUSIC="$GD/Data/streams/music"
PRX="$GD/Data/pre/qb_scripts.prx"
VAR="$ROOT/tools/bink/radio/variants"
PRISTINE="$ROOT/game-pristine-us/Data/streams/music"
ENTRY='scripts/game/skater/skater_sfx.qb'   # prx.py find() maps / -> \

swap_titles() {  # $1 = variant .qb
  python3 "$ROOT/tools/prx/prx.py" replacez "$PRX" "$ENTRY" "$1" "$PRX" >/dev/null
}

case "$MODE" in
  radio)
    [ -d "$ROOT/tools/bink/radio/bik" ] || { echo "no radio .bik (run encode_bik.sh)"; exit 1; }
    python3 "$ROOT/tools/bink/radio/apply_radio.py" "$GD" >/dev/null
    swap_titles "$VAR/skater_sfx_radio.qb"
    echo "soundtrack -> Violet Vandal Radio (royalty-free)"
    ;;
  original)
    cp "$PRISTINE"/*.bik "$MUSIC"/
    rm -f "$MUSIC/VIOLET_VANDAL_RADIO_credits.txt"
    swap_titles "$VAR/skater_sfx_original.qb"
    echo "soundtrack -> Original"
    ;;
  *) echo "usage: set_soundtrack.sh <gamedir> <radio|original>"; exit 2 ;;
esac
