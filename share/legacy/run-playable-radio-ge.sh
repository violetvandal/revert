#!/usr/bin/env bash
# THUG2: Violet Vandal Edition — ROYALTY-FREE RADIO lane (stream-safe).
# Same as run-playable-ge.sh but swaps in the CC-BY "Violet Vandal Radio"
# soundtrack (audio + jukebox titles) before launch. (A runtime in-game toggle
# is engine-impossible — see memory project_streaming_mode.)
set -euo pipefail
export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thug2-ge"
export WINEARCH=win32
export WINEDEBUG=-all
export PATH="$GE/bin:$PATH"

# Soundtrack lane: Violet Vandal Radio (royalty-free, CC-BY).
"$HOME/Documents/thug2/tools/bink/radio/set_soundtrack.sh" \
  "$HOME/Documents/thug2/game-playable-us" radio

# Start the trigger-bridge (LT->Nollie/KP7, RT->Switch/KP9).
BRIDGE="$HOME/Documents/thug2/tools/trigger-bridge/thug2-trigger-bridge.py"
bridge_pid=""
if [ -e "$BRIDGE" ]; then
  python3 "$BRIDGE" & bridge_pid=$!
  trap '[ -n "$bridge_pid" ] && kill "$bridge_pid" 2>/dev/null || true' EXIT
fi

cd "$HOME/Documents/thug2/game-playable-us"
"$GE/bin/wine" THUG2.exe "$@"
