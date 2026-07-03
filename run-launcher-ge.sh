#!/usr/bin/env bash
# Run the THUG2 config Launcher under the GE prefix to bind the controller.
# Bindings save to HKCU\Software\Neversoft in this prefix; the game reads them.
set -euo pipefail
export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thug2-ge"
export WINEARCH=win32
export WINEDEBUG=-all
export PATH="$GE/bin:$PATH"
cd "$HOME/Documents/thug2/game-modded-vanilla"
exec "$GE/bin/wine" Launcher.exe
