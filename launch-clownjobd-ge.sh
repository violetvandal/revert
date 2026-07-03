#!/usr/bin/env bash
# Launch the Clownjob'd profile under Lutris' Wine-GE 8-26 (wine-8.0 staging),
# in a dedicated prefix, to test whether controllers work where Wine 11 fails.
# Run this from an interactive terminal (NOT detached) so the game window keeps focus.
set -euo pipefail

export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thug2-ge"
export WINEARCH=win32
export WINEDEBUG=-all
export PATH="$GE/bin:$PATH"

cd "$HOME/Documents/thug2/game-modded-clownjobd"
exec "$GE/bin/wine" THUGTWO.exe
