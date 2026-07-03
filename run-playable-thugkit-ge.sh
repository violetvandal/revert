#!/usr/bin/env bash
# BOOT-TEST launcher for game-playable-thugkit — the install whose Data/pre was
# modded by the Go `thugkit` applier (vs the bash apply-mods.sh path). Same GE
# prefix + controller + trigger-bridge as run-playable-ge.sh; only the game dir
# differs. Clean empty Save/ so a stray .SKA can't cause a false black-screen.
set -euo pipefail
export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thug2-ge"
export WINEARCH=win32
export WINEDEBUG=-all
export PATH="$GE/bin:$PATH"

BRIDGE="$HOME/Documents/thug2/tools/trigger-bridge/thug2-trigger-bridge.py"
bridge_pid=""
if [ -e "$BRIDGE" ]; then
  python3 "$BRIDGE" & bridge_pid=$!
  trap '[ -n "$bridge_pid" ] && kill "$bridge_pid" 2>/dev/null || true' EXIT
fi

cd "$HOME/Documents/thug2/game-playable-thugkit"
"$GE/bin/wine" THUG2.exe "$@"
