#!/usr/bin/env bash
# Trace which music .bik files the game OPENS during playback, to diagnose the
# Original vs Violet Vandal Radio audio binding. WINEDEBUG=+file logs file I/O.
#
# STEPS while it runs:
#   1. Let the game reach the main menu.
#   2. Game Options -> MOD OPTIONS -> set "Soundtrack: VV Radio".
#   3. Start any level and let the jukebox play a track or two (~20s).
#   4. Quit the game.
# Then this prints which .bik checksums were opened (VVR_* vs original).
export GE="${GE:-$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64}"
export WINEPREFIX="$HOME/.wine-thug2-ge"
LOG=/tmp/thug2_music_trace.log
rm -f "$LOG"; touch "$LOG"
cd "$HOME/Documents/thug2/game-playable-us" || { echo "cd failed"; exit 1; }
echo "TRACING music file opens -> $LOG"
echo ">> In game: set Soundtrack=VV Radio, enter a level, let music play, then quit."
WINEDEBUG=+file "$GE/bin/wine" THUG2.exe "$@" 2>&1 >/dev/null \
  | grep --line-buffered -iE '\.bik|streams.music' \
  >> "$LOG"
echo "Done. Captured $(wc -l < "$LOG") lines."
echo "=== distinct .bik basenames opened ==="
grep -oiE '[0-9a-f]{8}\.bik' "$LOG" | tr 'A-F' 'a-f' | sort -u