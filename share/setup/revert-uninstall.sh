#!/usr/bin/env bash
#
# revert-uninstall.sh — remove what Revert installed (Linux / Steam Deck).
#
#   revert-uninstall.sh [--dry-run] [--yes] [--purge]
#     --dry-run   print the plan, remove nothing
#     --yes       skip the typed confirmation
#     --purge     FULL CLEAN: also delete saves (no backup), THUG Pro, the bootstrap
#                 Go toolchain, and the system packages setup installed
#
# Default depth removes the clone + built editions, the Wine prefixes, the Deck-installed
# Wine, the shortcuts, the registry bindings and the ~/.local/bin/revert symlink — after
# exporting every save + created tag to a dated backup folder. It KEEPS the bootstrap Go,
# shared system packages, and THUG Pro unless --purge is given.
#
# Safety: every path removed is either inside $REVERT_ROOT or on an explicit allowlist
# below; nothing above the clone is ever touched. Saves are exported BEFORE any deletion,
# so a failure there aborts with nothing lost. The clone removes itself LAST (bash keeps
# running from its open fd after the script file is unlinked).
#
set -uo pipefail   # NOT -e: a single failed rm must not abort the whole teardown

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

c_blue=$'\033[1;34m'; c_red=$'\033[1;31m'; c_grn=$'\033[1;32m'; c_yel=$'\033[1;33m'; c_dim=$'\033[2m'; c_off=$'\033[0m'
log()  { printf '%s[uninstall]%s %s\n' "$c_blue" "$c_off" "$*"; }
warn() { printf '%s[uninstall:warn]%s %s\n' "$c_yel" "$c_off" "$*" >&2; }
err()  { printf '%s[uninstall:error]%s %s\n' "$c_red" "$c_off" "$*" >&2; exit 1; }
gone() { printf '  %s✗%s %s\n' "$c_grn" "$c_off" "$*"; }
keep() { printf '  %s· %s%s\n' "$c_dim" "$*" "$c_off"; }

DRY=0; YES=0; PURGE=0
for a in "$@"; do case "$a" in
  --dry-run) DRY=1;;
  --yes)     YES=1;;
  --purge)   PURGE=1;;
  *) warn "ignoring unknown option: $a";;
esac; done

# ── sanity: never operate on a bad root ────────────────────────────────────────
[[ -n "$REVERT_ROOT" ]]                 || err "no toolkit root — refusing to uninstall"
[[ "$REVERT_ROOT" != "/" ]]             || err "toolkit root is / — refusing to uninstall"
[[ "$REVERT_ROOT" != "$HOME" ]]         || err "toolkit root is your home directory — refusing to uninstall"
[[ -f "$REVERT_ROOT/revert.conf" ]]     || err "$REVERT_ROOT is not a Revert install (no revert.conf) — refusing"

is_steam_deck() {
  [[ "${SteamDeck:-0}" == "1" ]] && return 0
  local pn=/sys/devices/virtual/dmi/id/product_name
  [[ -r "$pn" ]] && grep -qiE 'jupiter|galileo' "$pn" && return 0
  return 1
}
IS_DECK=0; is_steam_deck && IS_DECK=1

# Run as root, using the GUI's askpass helper when present (SUDO_ASKPASS), else a normal
# interactive sudo — same pattern revert-setup.sh uses.
ask_sudo() {
  if [[ -n "${SUDO_ASKPASS:-}" ]]; then sudo -A "$@"; else sudo "$@"; fi
}

# purge_packages removes ONLY the packages setup recorded in .revert-packages (the ones it
# actually installed, not ones that predated Revert), grouped by manager. Best-effort: a
# package another app now depends on will make the manager refuse, which is the safe outcome.
purge_packages() {
  local manifest="$REVERT_ROOT/.revert-packages"
  [[ -f "$manifest" ]] || return 0
  local -a dnf_pkgs=() pacman_pkgs=()
  local mgr pkg
  while read -r mgr pkg; do
    [[ -n "$mgr" && -n "$pkg" ]] || continue
    case "$mgr" in
      dnf)    dnf_pkgs+=("$pkg");;
      pacman) pacman_pkgs+=("$pkg");;
    esac
  done < "$manifest"
  if (( ${#dnf_pkgs[@]} )) && command -v dnf >/dev/null; then
    log "removing ${#dnf_pkgs[@]} dnf package(s) Revert installed (sudo)"
    ask_sudo dnf remove -y "${dnf_pkgs[@]}" && gone "dnf packages: ${dnf_pkgs[*]}" \
      || warn "some packages were kept (another app needs them) — that's fine"
  fi
  if (( ${#pacman_pkgs[@]} )) && command -v pacman >/dev/null; then
    (( IS_DECK )) && { ask_sudo steamos-readonly disable >/dev/null 2>&1 || true; }
    log "removing ${#pacman_pkgs[@]} pacman package(s) Revert installed (sudo)"
    ask_sudo pacman -Rns --noconfirm "${pacman_pkgs[@]}" && gone "pacman packages: ${pacman_pkgs[*]}" \
      || warn "some packages were kept (another app needs them) — that's fine"
  fi
}

# within DIR PATH — true iff PATH is DIR or lives under it (guards every removal).
within() {
  local dir="${1%/}" path="$2"
  [[ "$path" == "$dir" || "$path" == "$dir"/* ]]
}

# ── build the allowlist of absolute paths outside the clone we may remove ───────
ALLOW=("$HOME/.local/bin/revert" "$PREFIX_MAIN" "$PREFIX_ONLINE")
# The Deck-installed Wine ONLY. On the desktop GE_DIR points at wine-ge-8-26, which Revert
# did not install (setup only checks for it) — never remove that.
if (( IS_DECK )) && [[ "$GE_DIR" == *"/wine-11.11-staging-amd64" ]]; then
  ALLOW+=("$GE_DIR")
fi
ALLOW+=("$HOME/.local/share/applications/thug2-violet-vandal.desktop")
[[ -f /run/ostree-booted ]] && ALLOW+=("$HOME/.local/lib/revert-sdl32")
if (( PURGE )); then
  ALLOW+=("$REVERT_ROOT/game-thugpro")   # THUG Pro base, inside the clone anyway
fi

allowed() {  # allowed PATH — true iff PATH is inside the clone or the allowlist
  local p="$1" a
  within "$REVERT_ROOT" "$p" && return 0
  for a in "${ALLOW[@]}"; do [[ -n "$a" ]] && within "$a" "$p" && return 0; done
  return 1
}

# Collected as we plan; executed later so the whole plan is shown/confirmed first.
declare -a RM_PATHS=() RM_LABELS=()
plan_rm() {  # plan_rm PATH LABEL
  local p="$1" label="$2"
  [[ -e "$p" || -L "$p" ]] || return 0
  if ! allowed "$p"; then warn "refusing to plan $p (outside the install + allowlist)"; return 0; fi
  RM_PATHS+=("$p"); RM_LABELS+=("$label")
}

# ── saves: export first (default) or delete with the clone (--purge) ───────────
BACKUP_DIR=""
declare -a EXPORTS=()
SAVE_DIRS=("$EDITION_QOL/Save" "$EDITION_VANILLA/Save")
for s in "${SAVE_DIRS[@]}"; do [[ -d "$s" ]] && EXPORTS+=("$s"); done
# revert.conf.local is user config, not ours — treat it like a save (back up, then remove).
[[ -f "$REVERT_ROOT/revert.conf.local" ]] && EXPORTS+=("$REVERT_ROOT/revert.conf.local")

if (( ! PURGE )) && (( ${#EXPORTS[@]} )); then
  base="$HOME/thug2-saves-backup-$(date +%Y-%m-%d)"
  BACKUP_DIR="$base"; n=2
  while [[ -e "$BACKUP_DIR" ]]; do BACKUP_DIR="${base}-${n}"; ((n++)); done
fi

# ── plan the removals ──────────────────────────────────────────────────────────
# Prefixes + Deck wine + symlink + desktop launcher + OSTree shim (outside the clone).
plan_rm "$PREFIX_MAIN"   "main Wine prefix (Vanilla + QOL)"
plan_rm "$PREFIX_ONLINE" "online Wine prefix (THUG Pro)"
plan_rm "$HOME/.local/bin/revert" "the 'revert' command symlink"
plan_rm "$HOME/.local/share/applications/thug2-violet-vandal.desktop" "app-menu launcher"
if (( IS_DECK )) && [[ "$GE_DIR" == *"/wine-11.11-staging-amd64" ]]; then
  plan_rm "$GE_DIR" "Deck-installed Wine 11.11"
fi
[[ -f /run/ostree-booted ]] && plan_rm "$HOME/.local/lib/revert-sdl32" "OSTree 32-bit SDL2 shim"

# The clone LAST (so game-*/, tools/thugkit, gui/revert-gui, deck-run.log, play-qol.sh,
# .revert-cache all go with it, and bash keeps running from its unlinked fd).
plan_rm "$REVERT_ROOT" "the Revert toolkit folder (clone + built editions + game data)"

# ── things we deliberately keep (and why) ──────────────────────────────────────
declare -a KEEPS=()
if (( ! PURGE )); then
  [[ -d "$HOME/.local/go" ]] && KEEPS+=("the Go build tool ($HOME/.local/go) — shared; --purge removes it if Revert installed it")
  [[ -f "$REVERT_ROOT/.revert-packages" ]] && KEEPS+=("system packages Revert installed — shared with other apps; --purge removes them")
  [[ -d "$REVERT_ROOT/game-thugpro" ]] && KEEPS+=("THUG Pro (game-thugpro/) — a separate community app; --purge removes it")
fi
KEEPS+=("system libraries installed with your package manager (dnf/pacman) and /dev/uinput access")
KEEPS+=("your Steam Deck account password")

# ── the plan output ────────────────────────────────────────────────────────────
echo
if (( PURGE )); then
  log "${c_red}uninstall --purge — FULL CLEAN.${c_off} This will remove:"
else
  log "uninstall — this will remove:"
fi
for i in "${!RM_PATHS[@]}"; do
  printf '  %s✗%s %s\n      %s%s%s\n' "$c_red" "$c_off" "${RM_LABELS[$i]}" "$c_dim" "${RM_PATHS[$i]}" "$c_off"
done
if (( IS_DECK )); then
  printf '  %s✗%s Steam library shortcut + tile art (removed via Steam)\n' "$c_red" "$c_off"
fi
if (( PURGE )); then
  [[ -f "$REVERT_ROOT/.revert-installed-go" ]] && printf '  %s✗%s the Go build tool (Revert installed it)\n' "$c_red" "$c_off"
  [[ -f "$REVERT_ROOT/.revert-packages" ]] && printf '  %s✗%s system packages Revert installed (only the ones it added)\n' "$c_red" "$c_off"
fi
if [[ -n "$BACKUP_DIR" ]] && (( ${#EXPORTS[@]} )); then
  echo
  printf '  Your saves are backed up first, to:\n      %s%s%s\n' "$c_grn" "$BACKUP_DIR" "$c_off"
  for s in "${EXPORTS[@]}"; do printf '        %s· %s%s\n' "$c_dim" "$s" "$c_off"; done
elif (( PURGE )); then
  printf '  %s!%s saves and created tags will be DELETED, not backed up (--purge)\n' "$c_red" "$c_off"
fi
for k in "${KEEPS[@]}"; do keep "keeping: $k"; done

if (( DRY )); then
  echo; log "dry run — nothing was removed."
  exit 0
fi

# ── confirm ────────────────────────────────────────────────────────────────────
if (( ! YES )); then
  word="yes"; (( PURGE )) && word="PURGE"
  echo
  (( PURGE )) && printf '%sThis is a FULL CLEAN and your saves will NOT be kept.%s\n' "$c_red" "$c_off"
  printf 'Type %s%s%s to continue (anything else cancels): ' "$c_red" "$word" "$c_off"
  read -r reply
  [[ "$reply" == "$word" ]] || { log "cancelled."; exit 0; }
fi

# ── export saves BEFORE removing anything ──────────────────────────────────────
if [[ -n "$BACKUP_DIR" ]] && (( ${#EXPORTS[@]} )); then
  for s in "${EXPORTS[@]}"; do
    # Namespace by the edition/parent dir so the two Save/ dirs don't collide, and a
    # loose file (revert.conf.local) keeps its own name.
    if [[ -d "$s" ]]; then dest="$BACKUP_DIR/$(basename "$(dirname "$s")")/$(basename "$s")"
    else dest="$BACKUP_DIR/$(basename "$s")"; fi
    mkdir -p "$(dirname "$dest")" || err "cannot create backup dir — nothing removed"
    cp -a "$s" "$dest" || err "backing up $s failed — nothing has been removed"
  done
  gone "saves backed up to $BACKUP_DIR"
fi

# ── remove the Deck Steam shortcut (needs Steam closed; best-effort) ───────────
if (( IS_DECK )); then
  tool="$REVERT_ROOT/tools/deck/add-steam-shortcut.py"
  if [[ -f "$tool" ]] && command -v python3 >/dev/null; then
    if pgrep -x steam >/dev/null && ! pgrep -x gamescope >/dev/null; then
      steam -shutdown >/dev/null 2>&1 || true
      for _ in $(seq 1 20); do pgrep -x steam >/dev/null || break; sleep 1; done
    fi
    for nm in "Tony Hawk's Underground 2 (VV Edition)" "THUG2: Violet Vandal Edition"; do
      python3 "$tool" --name "$nm" --remove >/dev/null 2>&1 && gone "Steam shortcut removed: $nm" || true
    done
  else
    warn "Steam shortcut tool missing — remove the non-Steam game from your library by hand"
  fi
fi

# ── unset the controller registry bindings (prefix is about to go anyway; this is
#     only meaningful if a prefix somehow survives, e.g. a shared prefix) ────────
# (No separate action needed: pad0 + bindings live inside $PREFIX_MAIN, removed below.)

# ── remove everything EXCEPT the clone (which must go last) ─────────────────────
for i in "${!RM_PATHS[@]}"; do
  p="${RM_PATHS[$i]}"
  [[ "$p" == "$REVERT_ROOT" ]] && continue     # the clone is removed last
  if rm -rf "$p"; then gone "${RM_LABELS[$i]}"; else warn "could not remove ${RM_LABELS[$i]} ($p)"; fi
done

# ── purge extras: Go (if we installed it) + recorded system packages ───────────
if (( PURGE )); then
  if [[ -f "$REVERT_ROOT/.revert-installed-go" ]]; then
    go_path="$(head -n1 "$REVERT_ROOT/.revert-installed-go")"
    if [[ "$go_path" == "$HOME/.local/go" && -d "$go_path" ]]; then
      rm -rf "$go_path" && gone "Go build tool ($go_path)" || warn "could not remove $go_path"
    fi
  fi
  if [[ -f "$REVERT_ROOT/.revert-packages" ]]; then
    purge_packages
  fi
fi

# ── remove the clone LAST — cd out first, bash runs on from its open fd ─────────
cd / || cd "$HOME" || true
if rm -rf "$REVERT_ROOT"; then gone "the Revert toolkit folder ($REVERT_ROOT)"; else warn "could not fully remove $REVERT_ROOT"; fi

echo
log "uninstall complete."
if [[ -n "$BACKUP_DIR" ]] && (( ${#EXPORTS[@]} )); then
  log "Your saves are safe at: ${c_grn}$BACKUP_DIR${c_off}"
fi
exit 0
