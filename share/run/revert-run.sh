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
# shellcheck disable=SC1090
[[ -f "${REVERT_ROOT}/share/lib/gpu.sh" ]] && source "${REVERT_ROOT}/share/lib/gpu.sh"

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
    --gpu)        GPU_FILTER="${2:-}"; shift 2;;
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

# GPU selection: --gpu / GPU_FILTER (a deviceName substring) picks which Vulkan adapter
# DXVK renders on. Empty = DXVK's default (the discrete GPU). See share/lib/gpu.sh.
[[ -n "${GPU_FILTER:-}" ]] && export DXVK_FILTER_DEVICE_NAME="$GPU_FILTER"

# Force builtin dinput8 on the main lanes so DirectInput enumerates the gamepad. A
# native dinput8 (e.g. from an old `winetricks dinput8`) silently kills pad detection
# for THUG2. Self-heals prefixes not yet re-run through `revert setup`. Only names
# dinput8, so the registry winmm=native,builtin (WSFix) override still applies.
if [[ "$PREFIX" == "$PREFIX_MAIN" ]]; then
  export WINEDLLOVERRIDES="dinput8=b${WINEDLLOVERRIDES:+;$WINEDLLOVERRIDES}"
fi

# On OSTree-based distros (Bazzite, Silverblue, Kinoite) the system SDL2 library is
# 64-bit only, but Wine's 32-bit winebus.sys needs libSDL2-2.0.so.0 (i686) to enumerate
# gamepads — without it winebus defers every controller to SDL and then drops it.
# Symlink ONLY libSDL2 into a private dir rather than prepending the full Steam runtime
# to LD_LIBRARY_PATH — that runtime ships libvulkan.so.1 which shadows the system Vulkan
# ICD and causes DXVK to fail with "Failed to create Vulkan instance".
# Gate to OSTree/immutable distros (Bazzite, Silverblue, Kinoite) — only they lack a
# 32-bit system SDL2. SteamOS/Steam Deck and Fedora/Arch desktops already ship it, so we
# must NOT shadow it with the old Steam-runtime copy there (that would regress a working pad).
if [[ -f /run/ostree-booted ]]; then
  _sdl32_src="${HOME}/.local/share/Steam/ubuntu12_32/steam-runtime/usr/lib/i386-linux-gnu/libSDL2-2.0.so.0"
  _sdl32_dir="${HOME}/.local/lib/revert-sdl32"
  if [[ -f "${_sdl32_src}" ]]; then
    mkdir -p "${_sdl32_dir}"
    ln -sf "${_sdl32_src}" "${_sdl32_dir}/libSDL2-2.0.so.0"
  fi
  [[ -L "${_sdl32_dir}/libSDL2-2.0.so.0" ]] \
    && export LD_LIBRARY_PATH="${_sdl32_dir}${LD_LIBRARY_PATH:+:${LD_LIBRARY_PATH}}"
  unset _sdl32_src _sdl32_dir
fi

# On Bazzite/OSTree, z: -> / is a composefs overlay that reports 0 bytes free.
# Wine picks the longest-prefix drive match, so without a closer drive letter the
# game lives on z: and GetDiskFreeSpaceEx returns 0 → "not enough disk space for
# saves". Add d: pointing at REVERT_ROOT (on the real data partition) so Wine routes
# the game path through d: instead of z:, and the disk-space query returns real stats.
ln -sfn "$REVERT_ROOT" "${PREFIX}/dosdevices/d:"

# button-glyph style for VV.GlyphFix.asi (xbox/playstation/gamecube/keyboard)
VV_GLYPHS="$(resolve_glyphs "$GLYPHS")"; export VV_GLYPHS
log "button glyphs -> $VV_GLYPHS$( [[ "$GLYPHS" == auto ]] && is_steam_deck && echo ' (Steam Deck)')"

# One-time heads-up if this box has a GPU pick worth making (multi-GPU / nouveau / llvmpipe)
# and no filter is set. Advisory only — never blocks the launch.
declare -F gpu_advise >/dev/null && gpu_advise log || true

# hooks ------------------------------------------------------------------------
# Only the main prefix (vanilla/qol) gets wineserver management — the online lane
# hands off to THUG Pro's own launcher and must not have its server torn down.
bridge_pid=""
MANAGE_SERVER=0; [[ "$PREFIX" == "$PREFIX_MAIN" ]] && MANAGE_SERVER=1

cleanup() {
  [[ -n "$bridge_pid" ]] && kill "$bridge_pid" 2>/dev/null || true
  pkill -f 'thug2-trigger-bridge.py' 2>/dev/null || true
  pkill -f 'thug2-pad-mirror.py'     2>/dev/null || true
  # Settle the wineserver so the NEXT launch starts on a clean server. A server left
  # running with the game's stale input state is what makes a relaunch hang at boot
  # or come up with a dead controller.
  (( MANAGE_SERVER )) && timeout 15 "$GE_DIR/bin/wineserver" -w 2>/dev/null
  return 0
}
trap cleanup EXIT

# Clear leftovers from a previous session BEFORE launching (e.g. a game that didn't
# exit cleanly, or an orphaned input bridge) — otherwise wineserver contention makes
# the relaunch hang / lose the pad. Match THUG2.exe EXACTLY; never `pkill -f THUG2`
# (that would also match this launcher's own command line and kill ourselves).
if (( MANAGE_SERVER )); then
  pkill -9 -x THUG2.exe 2>/dev/null || true
  pkill -f 'thug2-trigger-bridge.py' 2>/dev/null || true
  pkill -f 'thug2-pad-mirror.py'     2>/dev/null || true
  timeout 10 "$GE_DIR/bin/wineserver" -w 2>/dev/null || true
fi

run_hook() {
  case "$1" in
    soundtrack)
      if [[ -x "$SET_SOUNDTRACK" ]]; then
        log "soundtrack -> $SOUNDTRACK"
        "$SET_SOUNDTRACK" "$DIR" "$SOUNDTRACK" || log "(soundtrack swap skipped — keeping the build's soundtrack)"
      else
        log "(set_soundtrack.sh absent — leaving current soundtrack)"
      fi;;
    padfix)
      # THUG2 only opens the gamepad whose DirectInput guidInstance matches the registry
      # value pad0 (HKCU\...\Settings). The GUID is assigned by Wine/SDL and can vary
      # between Wine versions and systems, so a static value in thug2-settings.reg goes
      # stale. Probe the live pad and refresh pad0 before each launch.
      #
      # Deck: pad0 points at the virtual "Violet Vandal Pad" whose GUID is pinned by
      # `revert calibrate-controller` (run once after build). Probing here would clobber
      # that with the raw Xbox GUID → skip on Deck.
      # Desktop: SDL (loaded via Steam runtime) assigns the GUID; probe it fresh each run.
      if ! is_steam_deck; then
        if [[ -f "${PAD_PROBE:-}" ]]; then
          local guid
          # `|| true` is load-bearing: under `set -euo pipefail`, a command-substitution
          # assignment inherits the pipeline's exit status, and pipefail makes the pipeline
          # non-zero whenever the wine probe exits non-zero — which it does on a cold wineserver
          # OR when no controller is attached. Without this guard that silently aborts the whole
          # launch (set -e) instead of falling through to the "no gamepad detected" branch below,
          # so `revert run` refuses to start with just a keyboard. Keep it.
          guid="$(WINEDEBUG=-all timeout 30 "$GE_DIR/bin/wine" "$PAD_PROBE" 2>/dev/null \
                  | awk '/-> GAMEPAD/{g=1} g&&/guidInstance=/{sub(/.*guidInstance=/,"");print;exit}' \
                  | tr -d '[:space:]')" || true
          if [[ "$guid" =~ ^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$ ]]; then
            WINEDEBUG=-all "$GE_DIR/bin/wine" reg add \
              "HKCU\\Software\\Activision\\Tony Hawk's Underground 2\\Settings" \
              /v pad0 /t REG_SZ /d "$guid" /f >/dev/null 2>&1 \
              && log "pad0 -> $guid" \
              || log "(padfix: reg write failed — pad0 unchanged)"
          else
            log "(padfix: no gamepad GUID detected — is the controller on? leaving pad0)"
          fi
        else
          log "(padfix: pad probe missing: ${PAD_PROBE:-unset})"
        fi
        # The probe started a wineserver. Flush + tear it fully down so THUG2 boots on
        # a clean server — stale input state from the probe hangs the game at boot.
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
# NOT `exec` — we must stay as the parent so the EXIT trap runs when the game quits
# (kills the input bridge + settles the wineserver). `exec` here silently orphaned
# the bridge and left the server up, which broke the next relaunch.
log "lane=$lane prefix=$PREFIX exe=$EXE"
cd "$DIR"
rc=0; "$GE_DIR/bin/wine" "$EXE" "$@" || rc=$?
log "game exited (code $rc) — cleaning up"
exit "$rc"
