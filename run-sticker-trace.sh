#!/usr/bin/env bash
# Trace which texture/image files the game opens around a Sticker Slap, to find
# where the slap sticker actually loads from (the loose Data/images/Tags/sticker
# .img.xbx replacement had no effect).
#
# STEPS while it runs:
#   1. Reach main menu, start any level.
#   2. Do a Sticker Slap on a wall.
#   3. Quit.
export GE="${GE:-$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64}"
export WINEPREFIX="$HOME/.wine-thug2-ge"
LOG=/tmp/thug2_sticker_trace.log
rm -f "$LOG"; touch "$LOG"
cd "$HOME/Documents/thug2/game-playable-us" || { echo "cd failed"; exit 1; }
echo "TRACING image/texture opens -> $LOG"
echo ">> Start a level, do a Sticker Slap, then quit."
WINEDEBUG=+file "$GE/bin/wine" THUG2.exe "$@" 2>&1 >/dev/null \
  | grep --line-buffered -iE 'sticker|cagpieces|cagr|tags|graffiti|\.img|\.tex|\.pre|\.prx|skaterparts' \
  >> "$LOG"
echo "Done. Captured $(wc -l < "$LOG") lines."
echo "=== distinct sticker/tag/cag image opens ==="
grep -oiE '[a-z0-9_\\]+\.(img|tex)(\.xbx)?' "$LOG" | sort -u | grep -iE 'sticker|tag|cag|tags' | head
echo "=== .pre / .prx archives opened ==="
grep -oiE '[a-z0-9_\\]+\.(pre|prx)' "$LOG" | sort -u | head -30
echo "=== any open of the loose Tags\\sticker.img.xbx? ==="
grep -i 'tags' "$LOG" | grep -i sticker | head