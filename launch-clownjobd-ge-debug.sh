#!/usr/bin/env bash
# DEBUG launch: Clownjob'd under Wine-GE 8-26 with XInput tracing.
# Plug the controller in BEFORE running (hotplug crashes the game).
# Run from your own terminal, reach the main menu, wiggle the LEFT STICK and
# press A/B a few times, then quit. Then tell Claude; it reads the trace.
set -euo pipefail

export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thug2-ge"
export WINEARCH=win32
# trace XInput + dinput device acquisition so we can see who grabs the pad
export WINEDEBUG=+xinput,+dinput
export PATH="$GE/bin:$PATH"

cd "$HOME/Documents/thug2/game-modded-clownjobd"
exec "$GE/bin/wine" THUGTWO.exe > "$HOME/Documents/thug2/ge-xinput-trace.log" 2>&1
