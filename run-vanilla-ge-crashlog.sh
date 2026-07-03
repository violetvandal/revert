#!/usr/bin/env bash
# Vanilla THUG2 + full crash backtrace capture. Reproduce the manual crash,
# then quit/let it crash; the backtrace lands in vanilla-crash.log.
export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thug2-ge"
export WINEARCH=win32
export WINEDEBUG=+seh,+ff      # exception backtraces + force-feedback trace
export PATH="$GE/bin:$PATH"
cd "$HOME/Documents/thug2/game-modded-vanilla"
"$GE/bin/wine" THUG2.exe > "$HOME/Documents/thug2/vanilla-crash.log" 2>&1
echo "exit=$? (backtrace in ~/Documents/thug2/vanilla-crash.log)"
