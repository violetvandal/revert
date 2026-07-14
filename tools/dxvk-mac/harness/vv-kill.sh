#!/bin/bash
WINE="/Applications/Wine Stable.app/Contents/Resources/wine/bin/wine"
WS="/Applications/Wine Stable.app/Contents/Resources/wine/bin/wineserver"
pkill -9 -f THUG2.exe 2>/dev/null
pkill -9 -f explorer 2>/dev/null
WINEPREFIX="$HOME/.wine-thug2-ws" "$WS" -k 2>/dev/null
sleep 1
echo "killed"
