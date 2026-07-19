#!/usr/bin/env bash
#
# revert-report.sh — collect a diagnostic bundle for a bug report (Linux / Steam Deck).
#
#   revert report [-o FILE] [--no-log]
#
# Prints the report to stdout AND saves it, so it can be copy-pasted straight into a
# GitHub issue or attached as a file. Read-only: it inspects, it never changes anything.
#
# WHAT THIS DELIBERATELY DOES NOT COLLECT
#   Nothing from inside the game data, no save files, no serial/licence keys, and no
#   filesystem paths that still contain the user's name — $HOME and the username are
#   redacted on the way out (see redact()). Someone pasting this into a public issue
#   should not be handing over their identity along with their GPU model.
#
set -euo pipefail

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

OUT=""
WANT_LOG=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output) OUT="${2:-}"; shift 2;;
    --no-log)    WANT_LOG=0; shift;;
    *) shift;;
  esac
done
[[ -n "$OUT" ]] || OUT="${PWD}/revert-report.txt"

RUN_LOG="${REVERT_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/revert}/last-run.log"

# ---- helpers -------------------------------------------------------------------
# redact strips the two things that identify a person: their home directory (which on
# most distros contains their login name) and the login name itself. Home goes first, so
# "/home/jane" becomes "~" rather than "/home/<user>".
#
# The username is matched on a word boundary, case-sensitively, and only when it is long
# enough to be distinctive. Over-redaction is its own failure mode: a user called "ati"
# would otherwise see every "HDA ATI HDMI" line rewritten, and a mangled report is worse
# than an unredacted one because it is wrong without looking wrong. (Same rule as the Go
# lane in internal/core/report.go, which has the test for it.)
redact() {
  local home="${HOME:-}" user="${USER:-${LOGNAME:-}}"
  local -a args=()
  [[ -n "$home" ]] && args+=(-e "s|${home}|~|g")
  [[ ${#user} -gt 2 ]] && args+=(-e "s|\\b${user}\\b|<user>|g")
  args+=(-e 's|/run/user/[0-9]*|/run/user/<uid>|g')
  sed "${args[@]}"
}

have() { command -v "$1" >/dev/null 2>&1; }
sec()  { printf '\n== %s ==\n' "$1"; }
kv()   { printf '%-22s %s\n' "$1" "${2:-(unknown)}"; }
yn()   { [[ -e "$1" ]] && echo yes || echo no; }

# first_line runs a command and returns one trimmed line, or "" if it is not installed
# or fails. Every probe here is best-effort: a missing tool must degrade to a blank
# field, never abort the report (which is exactly when someone needs it most).
first_line() { "$@" 2>/dev/null | head -n1 | sed 's/[[:space:]]*$//' || true; }

is_steam_deck() {
  [[ "${SteamDeck:-0}" == "1" ]] && return 0
  local pn=/sys/devices/virtual/dmi/id/product_name
  [[ -r "$pn" ]] && grep -qiE 'jupiter|galileo' "$pn" && return 0
  return 1
}

# ---- gather --------------------------------------------------------------------
generate() {
  echo "Revert diagnostic report"
  echo "(paths and usernames redacted; safe to paste publicly)"

  sec "Revert"
  kv "version"   "$(cd "$REVERT_ROOT" && git describe --tags --always 2>/dev/null || echo dev)"
  kv "root"      "$REVERT_ROOT"
  kv "branch"    "$(cd "$REVERT_ROOT" && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo n/a)"
  kv "dirty"     "$(cd "$REVERT_ROOT" && { [[ -n "$(git status --porcelain 2>/dev/null)" ]] && echo yes || echo no; })"

  sec "System"
  local distro=""
  [[ -r /etc/os-release ]] && distro="$(. /etc/os-release 2>/dev/null && echo "${PRETTY_NAME:-$NAME $VERSION_ID}")"
  kv "distro"    "$distro"
  kv "kernel"    "$(uname -r)"
  kv "arch"      "$(uname -m)"
  kv "steam deck" "$(is_steam_deck && echo yes || echo no)"
  kv "session"   "${XDG_SESSION_TYPE:-unknown} / ${XDG_CURRENT_DESKTOP:-unknown}"
  # An immutable/OSTree base (Bazzite, Silverblue) changes how packages install, and has
  # already been the root cause of one class of controller bug. Worth one line.
  kv "immutable fs" "$( [[ -d /ostree || -d /sysroot/ostree ]] && echo "yes (ostree)" || echo no)"

  sec "Hardware"
  kv "cpu"       "$(grep -m1 '^model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2- | sed 's/^ *//')"
  kv "cores"     "$(nproc 2>/dev/null)"
  kv "memory"    "$(free -h 2>/dev/null | awk '/^Mem:/{print $2" total, "$7" available"}')"
  kv "disk (root)" "$(df -h "$REVERT_ROOT" 2>/dev/null | awk 'NR==2{print $4" free of "$2}')"

  sec "Graphics"
  if have lspci; then
    lspci -nn 2>/dev/null | grep -Ei 'vga|3d|display' | sed 's/^/  /' || true
  else
    echo "  (lspci not installed)"
  fi
  kv "DXVK (configured)" "${DXVK_VERSION:-unset}"
  if have vulkaninfo; then
    echo "  vulkaninfo --summary:"
    # The summary block names each Vulkan device and its driver version, which is the
    # single most useful fact in a "it does not render" report.
    vulkaninfo --summary 2>/dev/null | sed -n '1,60p' | sed 's/^/    /' || true
  else
    echo "  vulkaninfo not installed (install vulkan-tools for the driver report)"
  fi
  if have glxinfo; then
    kv "GL renderer" "$(first_line sh -c 'glxinfo -B | grep -i "OpenGL renderer"')"
  fi

  sec "Wine runtime"
  kv "GE_DIR"      "$GE_DIR"
  kv "wine present" "$(yn "$GE_DIR/bin/wine")"
  [[ -x "$GE_DIR/bin/wine" ]] && kv "wine version" "$(first_line "$GE_DIR/bin/wine" --version)"
  kv "main prefix"   "$(yn "$PREFIX_MAIN")"
  kv "online prefix" "$(yn "$PREFIX_ONLINE")"

  sec "Toolchain"
  kv "go"       "$(first_line go version)"
  kv "python3"  "$(first_line python3 --version)"
  kv "thugkit"  "$(yn "$THUGKIT")"
  kv "evdev"    "$(python3 -c 'import evdev' 2>/dev/null && echo present || echo absent)"
  kv "/dev/uinput" "$( [[ -w /dev/uinput ]] && echo writable || echo "not writable")"

  # Presence only. Which files exist tells us where the lifecycle stopped; their
  # contents are the user's own game data and are none of our business.
  sec "Game data (presence only)"
  kv "pristine base" "$(yn "$PRISTINE_DIR/Data/pre")"
  kv "qol build"     "$(yn "${EDITION_QOL:-}/Data/pre")"
  kv "vanilla build" "$(yn "${EDITION_VANILLA:-}/Data/pre")"
  kv "thug pro"      "$(yn "${LANE_ONLINE_DIR:-/nonexistent}")"
  if [[ -f "$NOCD_EXE" ]] && have md5sum; then
    local sum; sum="$(md5sum "$NOCD_EXE" 2>/dev/null | cut -d' ' -f1)"
    # The shipped .asi mods hardcode addresses against one specific exe. A mismatch here
    # explains a whole family of "the HUD is in the wrong place" reports at a glance.
    if [[ "$sum" == "d464781a2863c833c640f7ff6d377ffe" ]]; then
      kv "THUG2.exe" "md5 matches the expected no-CD exe"
    else
      kv "THUG2.exe" "md5 $sum (DIFFERENT from the expected no-CD exe)"
    fi
  else
    kv "THUG2.exe" "not present"
  fi

  sec "Controllers seen by the kernel"
  # Only devices the kernel exposes as a JOYSTICK (Handlers contains js*). Listing every
  # input device instead buries the one useful line under the mice, the power button and
  # a dozen HDMI audio endpoints, and a report nobody reads helps nobody.
  if [[ -r /proc/bus/input/devices ]]; then
    local pads
    pads="$(awk '
      /^N: Name=/ { name = substr($0, 9) }
      /^H: Handlers=/ { if ($0 ~ /js[0-9]/) print "  " name }
    ' /proc/bus/input/devices 2>/dev/null || true)"
    if [[ -n "$pads" ]]; then
      echo "$pads"
    else
      echo "  (no joystick devices — is the controller plugged in and powered on?)"
    fi
  else
    echo "  (/proc/bus/input/devices unreadable)"
  fi

  if (( WANT_LOG )); then
    sec "Last run (tail)"
    if [[ -f "$RUN_LOG" ]]; then
      kv "log file" "$RUN_LOG"
      echo
      tail -n 200 "$RUN_LOG" | sed 's/^/  /'
    else
      echo "  No run log yet. Launch the game once (revert run qol), then re-run this."
    fi
  fi
}

generate 2>&1 | redact > "$OUT"

cat "$OUT"
printf '\n\033[1;34m[revert]\033[0m saved to %s\n' "$OUT"
echo "Open an issue and attach it: https://github.com/violetvandal/revert/issues/new/choose"
echo "Read it before you post if you like — it is plain text, and nothing in it is secret."
