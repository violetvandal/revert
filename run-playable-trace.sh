#!/usr/bin/env bash
# Diagnostic launcher: logs every file the game opens (filtered to skater/texture
# assets) so we can see exactly where the body/panty texture is loaded from.
export GE="${GE:-$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64}"
export WINEPREFIX="$HOME/.wine-thug2-ge"
LOG=/tmp/thug2_filetrace.log
rm -f "$LOG"; touch "$LOG"
cd "$HOME/Documents/thug2/game-playable-us" || { echo "cd failed"; exit 1; }
echo "TRACING -> $LOG"
echo "Play until your skater is on screen, then quit the game."
# stderr (wine +file debug) -> pipe -> grep -> log ; stdout -> discarded
WINEDEBUG=+file "$GE/bin/wine" THUG2.exe "$@" 2>&1 >/dev/null \
  | grep --line-buffered -iE 'skater|tempprofile|\.tex|\.img|\.skin|skaterparts|\\cas|female' \
  >> "$LOG"
echo "Done. Captured $(wc -l < "$LOG") lines -> $LOG"
