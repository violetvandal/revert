#!/usr/bin/env bash
#
# revert-run.sh — config-driven lane launcher for THUG2: Violet Vandal Edition.
# Replaces the loose run-*.sh zoo. Each lane is defined in revert.conf as
#   LANE_<NAME>_{DIR,PREFIX,EXE,ENV,HOOKS,SOUNDTRACK}
#
#   revert-run.sh <vanilla|qol|online> [--soundtrack original|radio] [-- extra wine args]
#
# Hooks (comma-separated in LANE_*_HOOKS):
#   soundtrack      pre-launch: swap Original/Radio music (QOL lane; engine can't toggle live)
#   padfix          Steam Deck: point THUG2's pad0 at the live controller's DInput GUID
#   trigger-bridge  start the evdev L2/R2 trigger bridge, kill it on exit
#
set -euo pipefail

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

log() { printf '\033[1;34m[run]\033[0m %s\n' "$*"; }
err() { printf '\033[1;31m[run:error]\033[0m %s\n' "$*" >&2; exit 1; }

# Steam Deck? (Game-Mode env, or DMI board name Jupiter=LCD / Galileo=OLED)
is_steam_deck() {
  [[ "${SteamDeck:-0}" == "1" ]] && return 0
  local pn=/sys/devices/virtual/dmi/id/product_name
  [[ -r "$pn" ]] && grep -qiE 'jupiter|galileo' "$pn" && return 0
  return 1
}
# Resolve the button-glyph style: explicit wins; "auto" -> xbox (Deck + the common Xbox-style
# pads). Returns one of: keyboard|xbox|playstation|gamecube. The .asi reads it via $VV_GLYPHS.
resolve_glyphs() {
  local s; s="$(echo "${1:-auto}" | tr '[:upper:]' '[:lower:]')"
  case "$s" in
    keyboard|xbox|playstation|gamecube) echo "$s";;
    ps|ps2)  echo playstation;;
    gc|ngc)  echo gamecube;;
    auto|"") if is_steam_deck; then echo xbox; else echo xbox; fi;;
    *) log "unknown glyph style '$s' — using xbox" >&2; echo xbox;;
  esac
}

lane="${1:-}"; shift || true
[[ -n "$lane" ]] || err "usage: revert-run.sh <vanilla|qol|online> [--soundtrack original|radio]"
UP="$(echo "$lane" | tr '[:lower:]' '[:upper:]')"

# resolve lane fields by indirection (LANE_QOL_DIR, ...)
dir_v="LANE_${UP}_DIR";   DIR="${!dir_v:-}"
pfx_v="LANE_${UP}_PREFIX"; PREFIX="${!pfx_v:-$PREFIX_MAIN}"
exe_v="LANE_${UP}_EXE";   EXE="${!exe_v:-}"
env_v="LANE_${UP}_ENV";   LANE_ENV="${!env_v:-}"
hk_v="LANE_${UP}_HOOKS";  HOOKS="${!hk_v:-}"
st_v="LANE_${UP}_SOUNDTRACK"; SOUNDTRACK="${!st_v:-original}"
GLYPHS="${GLYPH_STYLE:-auto}"
[[ -n "$DIR" && -n "$EXE" ]] || err "unknown lane '$lane' (use: vanilla | qol | online)"

# options
while [[ $# -gt 0 ]]; do
  case "$1" in
    --soundtrack) SOUNDTRACK="${2:-original}"; shift 2;;
    --glyphs)     GLYPHS="${2:-auto}"; shift 2;;
    --) shift; break;;
    *) break;;
  esac
done

[[ -x "$GE_DIR/bin/wine" ]] || err "GE-Proton wine missing: $GE_DIR (run: revert setup)"
[[ -d "$DIR" ]] || err "lane dir missing: $DIR (run: revert build, or revert setup for online)"

# wine environment
export WINEPREFIX="$PREFIX"
export WINEARCH="$WINEARCH"
export WINEDEBUG="-all"
export PATH="$GE_DIR/bin:$PATH"
[[ -n "$LANE_ENV" ]] && export "${LANE_ENV?}"   # e.g. WINEDLLOVERRIDES=mscoree=b

# button-glyph style for VV.GlyphFix.asi (xbox/playstation/gamecube/keyboard)
VV_GLYPHS="$(resolve_glyphs "$GLYPHS")"; export VV_GLYPHS
log "button glyphs -> $VV_GLYPHS$( [[ "$GLYPHS" == auto ]] && is_steam_deck && echo ' (Steam Deck)')"

# hooks ------------------------------------------------------------------------
bridge_pid=""
cleanup() { [[ -n "$bridge_pid" ]] && kill "$bridge_pid" 2>/dev/null || true; }
trap cleanup EXIT

run_hook() {
  case "$1" in
    soundtrack)
      if [[ -x "$SET_SOUNDTRACK" ]]; then
        log "soundtrack -> $SOUNDTRACK"
        "$SET_SOUNDTRACK" "$DIR" "$SOUNDTRACK" || err "soundtrack swap failed"
      else
        log "(set_soundtrack.sh absent — leaving current soundtrack)"
      fi;;
    padfix)
      # THUG2 only opens the gamepad whose DirectInput guidInstance matches the registry
      # value pad0 (HKCU\...\Settings). On the Steam Deck the controller is Steam Input's
      # emulated Xbox pad, and Wine can regenerate that GUID across reboots / Steam updates,
      # so a static pad0 goes stale and the game silently falls back to keyboard+mouse only.
      # Detect the live pad's GUID and write pad0 fresh before each launch. Deck-only; desktop
      # uses the physical pad's saved GUID. The probe runs Wine (dinput/winebus, opens the pad)
      # so it MUST tear its wineserver down before the game launches, or the game hangs at boot.
      if is_steam_deck; then
        if [[ -f "${PAD_PROBE:-}" ]]; then
          local guid
          guid="$(WINEDEBUG=-all timeout 30 "$GE_DIR/bin/wine" "$PAD_PROBE" 2>/dev/null \
                  | awk '/-> GAMEPAD/{g=1} g&&/guidInstance=/{sub(/.*guidInstance=/,"");print;exit}' \
                  | tr -d '[:space:]')"
          if [[ "$guid" =~ ^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$ ]]; then
            WINEDEBUG=-all "$GE_DIR/bin/wine" reg add \
              "HKCU\\Software\\Activision\\Tony Hawk's Underground 2\\Settings" \
              /v pad0 /t REG_SZ /d "$guid" /f >/dev/null 2>&1 \
              && log "pad0 -> $guid (live Deck controller)" \
              || log "(padfix: reg write failed — pad0 unchanged)"
          else
            log "(padfix: no gamepad GUID detected — is the controller on? leaving pad0)"
          fi
        else
          log "(padfix: pad probe missing: ${PAD_PROBE:-unset})"
        fi
        # Critical: the probe started a wineserver and opened the pad device. Flush + tear it
        # fully down so THUG2 boots on a clean server instead of inheriting the probe's
        # half-initialised input state (which hangs at the blue screen). -w flushes the pad0
        # write on graceful exit; then nuke any orphaned wine helpers it left behind.
        timeout 15 "$GE_DIR/bin/wineserver" -w 2>/dev/null || true
        pkill -9 -x services.exe  2>/dev/null || true
        pkill -9 -x winedevice.exe 2>/dev/null || true
        pkill -9 -x explorer.exe  2>/dev/null || true
        sleep 1
      fi;;
    trigger-bridge)
      # Combos THUG2 can't bind natively (LB+RB get-off) + trigger/bumper rotations.
      # Deck: the dependency-free pad bridge (stdlib only) reads Steam Input's emulated pad
      # and emits the game's keyboard keys via uinput — runs ALONGSIDE Steam Input (it does
      # not grab the pad), and self-terminates with the game (PR_SET_PDEATHSIG). The /dev/uinput
      # ACL grants the deck user access, so this works in Gaming Mode too.
      # Desktop: the original evdev bridge for the physical pad.
      if is_steam_deck; then
        if [[ -f "$PAD_BRIDGE" ]] && command -v python3 >/dev/null; then
          # kill any leftover mirror so our virtual pad is the FIRST instance (its Wine GUID,
          # which pad0 is pinned to, is only deterministic when it's the sole "Violet Vandal Pad")
          pkill -f thug2-pad-mirror.py 2>/dev/null || true; sleep 0.3
          python3 "$PAD_BRIDGE" & bridge_pid=$!
          # Wait for our virtual analog pad to register, so THUG2 enumerates+binds it at
          # startup (pad0 points at it). It isolates the game from Steam's flaky emulated
          # pad — the actual fix for the mid-game analog-input stall.
          for _ in $(seq 1 30); do
            grep -q "Violet Vandal Pad" /proc/bus/input/devices 2>/dev/null && break
            sleep 0.2
          done
          log "pad mirror started (pid $bridge_pid) — stable virtual analog pad + combos"
        else
          log "(pad mirror unavailable — sticks/buttons only)"
        fi
      elif [[ -f "$TRIGGER_BRIDGE" ]] && command -v python3 >/dev/null; then
        python3 "$TRIGGER_BRIDGE" & bridge_pid=$!
        log "trigger bridge started (pid $bridge_pid)"
      else
        log "(trigger bridge unavailable — native pad only)"
      fi;;
    "" ) ;;
    *) log "unknown hook '$1' (ignored)";;
  esac
}
IFS=',' read -ra _hooks <<< "$HOOKS"
for h in "${_hooks[@]}"; do run_hook "$h"; done

# launch -----------------------------------------------------------------------
log "lane=$lane prefix=$PREFIX exe=$EXE"
cd "$DIR"
exec "$GE_DIR/bin/wine" "$EXE" "$@"
