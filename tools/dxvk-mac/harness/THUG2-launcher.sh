#!/bin/bash
export WINEPREFIX="$HOME/.wine-thug2-ws"
export WINEDEBUG="fixme-all,err-all,warn-all"
export WINEDLLOVERRIDES="mscoree,mshtml=;d3d9=n,b;winmm=n,b;dinput8=n,b"
export MVK_CONFIG_LOG_LEVEL=1
WINE="/Applications/Wine Stable.app/Contents/Resources/wine/bin/wine"
cd "$HOME/THUG2" || exit 1
# bridge (XInput, vv-padbridge) + game launched inside ONE virtual desktop via vv-run.bat
# (SendInput is desktop-scoped). dinput8=n,b loads our left-stick de-inverter proxy.
"$WINE" explorer /desktop=thug2,1440x900 cmd /c vv-run.bat > "$HOME/thug2-menu.log" 2>&1
pkill -f vv-padbridge 2>/dev/null
