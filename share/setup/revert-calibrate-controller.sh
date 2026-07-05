#!/usr/bin/env bash
#
# revert-calibrate-controller.sh — Steam Deck: detect the virtual pad's DirectInput
# instance GUID and bind THUG2's pad0 to it.
#
# Why this exists: Wine 11.11 (SteamOS) assigns each HID joystick a DInput instance GUID
# of the form {XXXXXXXX-YYYY-11F1-800N-000044455354}, where XXXXXXXX-YYYY is regenerated
# for every fresh Wine prefix — so it CANNOT be hardcoded. The standalone GUID probe hangs
# forever on Wine 11.11, but the game's OWN +dinput enumeration does not. So: launch the
# built game briefly with DInput tracing, read our pad's guidInstance from the trace, and
# write pad0. One-time — the GUID is stable for the life of the prefix. Re-run any time the
# controller stops binding (e.g. after recreating the prefix): `revert calibrate-controller`.
set -uo pipefail

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

log()  { printf '\033[1;34m[calibrate]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[calibrate:warn]\033[0m %s\n' "$*" >&2; }

is_deck() { [[ "${SteamDeck:-0}" == "1" ]] || grep -qiE 'jupiter|galileo' \
  /sys/devices/virtual/dmi/id/product_name 2>/dev/null; }

wine="$GE_DIR/bin/wine"; wineserver="$GE_DIR/bin/wineserver"

# Preconditions (all non-fatal — calibration is a best-effort convenience).
is_deck                              || { log "not a Steam Deck — no pad calibration needed"; exit 0; }
[[ -x "$wine" ]]                     || { warn "wine missing ($wine)"; exit 0; }
[[ -d "${EDITION_QOL}/Data/pre" && -f "${EDITION_QOL}/THUG2.exe" ]] \
                                     || { warn "no built edition yet (run: revert build)"; exit 0; }
[[ -f "$PAD_BRIDGE" ]] && command -v python3 >/dev/null \
                                     || { warn "pad-mirror/python3 unavailable — cannot calibrate"; exit 0; }
export DISPLAY="${DISPLAY:-:0}"   # needs a desktop; the game window flashes briefly

cleanup() {
  kill -9 "$(pgrep -x THUG2.exe)" 2>/dev/null || true
  pkill -9 -f thug2-pad-mirror.py 2>/dev/null || true
  WINEPREFIX="$PREFIX_MAIN" timeout 20 "$wineserver" -k 2>/dev/null || true
}
trap cleanup EXIT

log "detecting the controller GUID — a game window will flash on screen for ~15s, leave it."
cleanup; sleep 1                                   # fresh state: no game, one vpad, fresh server
setsid python3 "$PAD_BRIDGE" >/dev/null 2>&1 </dev/null &
for _ in $(seq 1 25); do grep -q "Violet Vandal Pad" /proc/bus/input/devices 2>/dev/null && break; sleep 0.2; done

trace="$(mktemp)"
( cd "$EDITION_QOL" && WINEPREFIX="$PREFIX_MAIN" WINEDEBUG=+dinput \
    timeout 50 "$wine" THUG2.exe >"$trace" 2>&1 ) &

# Poll the trace until our pad's (VID 1209:764A) instance GUID shows up.
guid=""
for _ in $(seq 1 45); do
  guid="$(grep -i '1209:764a' "$trace" 2>/dev/null \
          | grep -oiE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)"
  [[ -n "$guid" ]] && break
  sleep 1
done
rm -f "$trace"

re='^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$'
if [[ "$guid" =~ $re ]]; then
  cleanup; sleep 1
  guid="${guid^^}"                                  # THUG2 stores pad0 uppercase, no braces
  WINEPREFIX="$PREFIX_MAIN" timeout 25 "$wine" reg add \
    "HKCU\\Software\\Activision\\Tony Hawk's Underground 2\\Settings" \
    /v pad0 /t REG_SZ /d "$guid" /f >/dev/null 2>&1 \
    && log "pad0 -> $guid (detected this prefix's virtual-pad GUID)" || warn "pad0 write failed"
else
  warn "couldn't read the pad GUID from the calibration launch — controller may need"
  warn "manual binding. Re-try with: revert calibrate-controller"
fi
