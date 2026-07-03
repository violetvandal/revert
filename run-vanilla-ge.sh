#!/usr/bin/env bash
# Vanilla THUG2 (GE prefix) + native analog controller + trigger-bridge for L2/R2.
set -euo pipefail
export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thug2-ge"
export WINEARCH=win32
export WINEDEBUG=-all
export PATH="$GE/bin:$PATH"

# Start the trigger-bridge (LT->Nollie/KP7, RT->Switch/KP9). Cloned Clownjob'd behavior.
BRIDGE="$HOME/Documents/thug2/tools/trigger-bridge/thug2-trigger-bridge.py"
bridge_pid=""
if [ -e "$BRIDGE" ]; then
  python3 "$BRIDGE" & bridge_pid=$!
  # stop the bridge whenever the game exits
  trap '[ -n "$bridge_pid" ] && kill "$bridge_pid" 2>/dev/null || true' EXIT
fi

cd "$HOME/Documents/thug2/game-modded-vanilla"
"$GE/bin/wine" THUG2.exe "$@"
