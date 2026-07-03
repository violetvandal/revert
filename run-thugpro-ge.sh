#!/usr/bin/env bash
# THUG Pro (beta 0.7.0) — separate optional profile, isolated from our modded THUG2.
# Total-conversion mod: every-game levels + online multiplayer. Installed to its OWN
# Wine prefix (~/.wine-thugpro); reads clean THUG2 base data from game-thugpro/ (never our mods).
# Uses the same GE-Proton wine runner as run-playable-ge.sh.
#
# Usage:
#   ./run-thugpro-ge.sh            # launcher/config GUI (resolution, controller bind) -> Play
#   ./run-thugpro-ge.sh --game     # skip the launcher, run the game directly with saved settings
set -euo pipefail
export GE="$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64"
export WINEPREFIX="$HOME/.wine-thugpro"
export WINEARCH=win32
export WINEDEBUG=-all
export WINEDLLOVERRIDES="mscoree=b"   # use builtin mscoree -> bundled Mono (.NET launcher)
export PATH="$GE/bin:$PATH"

APP="$WINEPREFIX/drive_c/users/$USER/AppData/Local/THUG Pro"

# THUG Pro has native gamepad binding (Gamepad Binding tab) + a "Disable XInput Device
# Trigger Split" option, so our base-THUG2 trigger-bridge is NOT used here.

cd "$APP"
if [ "${1:-}" = "--game" ]; then
  shift
  exec "$GE/bin/wine" "THUGPro.exe" "$@"
else
  exec "$GE/bin/wine" "THUGProLauncher.exe" "$@"
fi
