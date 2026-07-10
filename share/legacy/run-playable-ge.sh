#!/usr/bin/env bash
# Playable THUG2 built from the verified clean US rip (game-playable-us).
# Uses the proven GE prefix + native analog controller + L2/R2 trigger-bridge.
# Base = game-pristine-us (clean rip) with no-CD exe + WSFix winmm proxy applied.
set -euo pipefail
export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thug2-ge"
export WINEARCH=win32
export WINEDEBUG=-all
export PATH="$GE/bin:$PATH"

# Soundtrack lane: Original (licensed). See run-playable-radio-ge.sh for the
# royalty-free "Violet Vandal Radio" lane. Engine binds the jukebox at boot, so
# the choice must be set before launch (not toggleable in-game).
"$HOME/Documents/thug2/tools/bink/radio/set_soundtrack.sh" \
  "$HOME/Documents/thug2/game-playable-us" original

# Start the trigger-bridge (LT->Nollie/KP7, RT->Switch/KP9). Cloned Clownjob'd behavior.
BRIDGE="$HOME/Documents/thug2/tools/trigger-bridge/thug2-trigger-bridge.py"
bridge_pid=""
if [ -e "$BRIDGE" ]; then
  python3 "$BRIDGE" & bridge_pid=$!
  trap '[ -n "$bridge_pid" ] && kill "$bridge_pid" 2>/dev/null || true' EXIT
fi

# Button-prompt glyph style for VV.GlyphFix.asi (xbox|playstation|gamecube|keyboard).
# Override per launch: `VV_GLYPHS=playstation ./run-playable-ge.sh`. (revert run has --glyphs.)
export VV_GLYPHS="${VV_GLYPHS:-xbox}"

cd "$HOME/Documents/thug2/game-playable-us"
"$GE/bin/wine" THUG2.exe "$@"
