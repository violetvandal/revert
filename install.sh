#!/usr/bin/env bash
#
# install.sh — one-command installer for THUG2: Violet Vandal Edition.
#
#   bash <(curl -fsSL https://raw.githubusercontent.com/violetvandal/revert/main/install.sh)
#
# Run it exactly like that — NOT `curl … | bash`. Piping makes this script bash's stdin,
# so the one-time password/sudo prompt can't read your keyboard (there's a guard below
# that refuses the pipe form for this reason).
#
# Written for a first-timer on a Steam Deck (works on any Linux desktop too). It gets
# you from a fresh machine to playing with as little typing as possible:
#   1. makes sure you have an account password (fresh Decks have none — the one system
#      step needs it) and helps you set one if not,
#   2. fetches the Revert toolkit (git + submodules),
#   3. installs the Go build tool locally if it's missing (no admin needed),
#   4. runs system setup (Wine, controller, Steam shortcut),
#   5. downloads YOUR THUG2 copy from a link you paste, and builds the edition.
#
# Revert ships TOOLING, never game data — you must own THUG2 and supply your own copy.
set -uo pipefail

REPO_URL="${REVERT_REPO:-https://github.com/violetvandal/revert.git}"
DEST="${REVERT_DIR:-$HOME/thug2}"
GO_VER="${REVERT_GO_VER:-go1.23.5}"
TTY=/dev/tty; [[ -e "$TTY" ]] || TTY=/dev/stdin
# DRIVEN=1 → this run is driven by the GUI installer: no terminal, no keyboard prompts.
# The password and game source arrive via REVERT_PASSWORD / REVERT_GAME_SRC, and sudo is
# fed non-interactively through SUDO_ASKPASS (set by the GUI). See gui/main.go.
DRIVEN="${REVERT_DRIVEN:-0}"

p=$'\033[1;35m'; g=$'\033[1;32m'; y=$'\033[1;33m'; r=$'\033[1;31m'; d=$'\033[2m'; o=$'\033[0m'
step() { printf '\n%s==>%s %s\n' "$p" "$o" "$*"; }
ok()   { printf '  %s✓%s %s\n' "$g" "$o" "$*"; }
info() { printf '  %s· %s%s\n' "$d" "$*" "$o"; }
warn() { printf '  %s!%s %s\n' "$y" "$o" "$*"; }
die()  { printf '\n%s✗ %s%s\n' "$r" "$*" "$o" >&2; exit 1; }
ask()  { local a; printf '%s?%s %s ' "$p" "$o" "$1" >"$TTY"; read -r a <"$TTY" || a=""; printf '%s' "$a"; }

is_deck() { [[ "${SteamDeck:-0}" == "1" ]] || grep -qiE 'jupiter|galileo' \
  /sys/devices/virtual/dmi/id/product_name 2>/dev/null; }

printf '%s\n' "${p}"
cat <<'BANNER'
  ┌──────────────────────────────────────────────────┐
  │   THUG2: Violet Vandal Edition — installer         │
  │   Tony Hawk's Underground 2, modernized            │
  └──────────────────────────────────────────────────┘
BANNER
printf '%s' "${o}"
is_deck && info "Steam Deck detected." || info "Running on a Linux desktop."

# ── 0. must have a real terminal ──────────────────────────────────────────────
# Piping into bash (curl … | bash) makes the script itself bash's stdin, so the
# password/sudo prompts would read the script instead of your keyboard. Require a
# terminal and tell the user the right, still-one-line way to run it.
if [[ "$DRIVEN" != 1 && ! -t 0 ]]; then
  die "Run this from a terminal so it can read your keyboard (for the password step).
  Use this one line instead of piping:

      bash <(curl -fsSL <the-installer-URL>)

  …or download then run:
      curl -fsSL <the-installer-URL> -o install.sh && bash install.sh"
fi

# ── 1. git ───────────────────────────────────────────────────────────────────
step "Checking for git"
command -v git >/dev/null || die "git isn't installed. On the Steam Deck it's built in; \
otherwise install 'git' with your package manager, then re-run this."
ok "git present"

# ── 2. account password (fresh SteamOS 'deck' user has none; setup needs sudo) ─
step "Checking your account password"
pwstat="$(passwd -S 2>/dev/null | awk 'NR==1{print $2}')"
if [[ "$pwstat" == "NP" || "$pwstat" == "L" ]]; then
  if [[ "$DRIVEN" == 1 ]]; then
    [[ -n "${REVERT_PASSWORD:-}" ]] || die "no password provided (setup needs one to install a few system libraries)."
    info "Setting your account password (needed for the one system step)."
    # passwd reads from a controlling terminal, not a pipe — hand it a PTY via util-linux
    # `script` and feed the new password twice. A fresh 'NP' account isn't asked for a
    # current password, so two lines suffice.
    if printf '%s\n%s\n' "$REVERT_PASSWORD" "$REVERT_PASSWORD" | script -qec passwd /dev/null >/dev/null 2>&1; then
      ok "password set"
    else
      die "couldn't set your password automatically. Open Konsole and run 'passwd' once, then re-run the installer."
    fi
  else
    warn "You don't have a password set yet — the setup step needs one (it installs a"
    info "few system libraries). Let's set it now. Type a new password twice; nothing"
    info "shows on screen as you type. Remember it — you'll use it for the setup step."
    passwd </dev/tty || die "couldn't set a password. Run 'passwd' yourself, then re-run this installer."
    ok "password set"
  fi
else
  ok "password is set"
fi

# ── 3. Go (only needed to build; install locally, no admin) ───────────────────
step "Checking for the Go build tool"
if command -v go >/dev/null; then
  ok "Go present ($(go version | awk '{print $3}'))"
elif [[ -x "$HOME/.local/go/bin/go" ]]; then
  export PATH="$HOME/.local/go/bin:$PATH"; ok "Go present (local)"
else
  info "Go isn't installed — fetching it locally (one-time, no admin needed)."
  arch=amd64; case "$(uname -m)" in aarch64|arm64) arch=arm64;; esac
  tgz="${GO_VER}.linux-${arch}.tar.gz"
  mkdir -p "$HOME/.local"
  if command -v curl >/dev/null; then curl -fL "https://go.dev/dl/$tgz" -o "/tmp/$tgz" || die "Go download failed"
  else wget -O "/tmp/$tgz" "https://go.dev/dl/$tgz" || die "Go download failed"; fi
  rm -rf "$HOME/.local/go"
  tar -C "$HOME/.local" -xzf "/tmp/$tgz" || die "Go extract failed"; rm -f "/tmp/$tgz"
  export PATH="$HOME/.local/go/bin:$PATH"
  command -v go >/dev/null && ok "Go installed ($(go version | awk '{print $3}'))" || die "Go install failed"
fi

# ── 4. fetch the toolkit ──────────────────────────────────────────────────────
step "Fetching the Revert toolkit"
if [[ -d "$DEST/.git" ]]; then
  info "already have it at $DEST — updating"
  git -C "$DEST" pull --recurse-submodules --ff-only || warn "update skipped (local changes?)"
  git -C "$DEST" submodule update --init --recursive || true
  ok "up to date: $DEST"
else
  git clone --recursive "$REPO_URL" "$DEST" || die "clone failed (network?)"
  ok "installed to $DEST"
fi
cd "$DEST" || die "cannot enter $DEST"

# Put `revert` on your PATH so it runs from any folder (not just $DEST). The dispatcher
# resolves its own root via `readlink -f`, so a symlink works correctly.
mkdir -p "$HOME/.local/bin"
if ln -sf "$DEST/revert" "$HOME/.local/bin/revert" 2>/dev/null; then
  ok "'revert' is now runnable from anywhere (~/.local/bin/revert)"
  case ":$PATH:" in
    *":$HOME/.local/bin:"*) : ;;
    *) info "add ~/.local/bin to your PATH to use the short 'revert' (or run $DEST/revert)";;
  esac
fi

# ── 5. system setup (Wine, controller, Steam shortcut) ────────────────────────
step "System setup — Wine, controller, Steam shortcut"
info "This is the step that needs your password (to install a few 32-bit libraries)."
./revert setup || die "setup failed — see the messages above."

# ── 6. game data — bring your own copy (paste a link) ─────────────────────────
step "Your THUG2 game files"
if ./revert status --json 2>/dev/null | grep -q '"pristine":true'; then
  ok "game files already in place"
else
  info "Revert ships no game data — point it at YOUR copy of THUG2."
  if [[ "$DRIVEN" == 1 ]]; then
    src="${REVERT_GAME_SRC:-}"
  else
    info "Paste a download link to a .zip / .7z / .iso of your game, or a folder path."
    src="$(ask 'game source (URL or folder path):')"
  fi
  [[ -n "$src" ]] || die "no source given. When ready, run:  ./revert acquire-game-data --url <link>"
  case "$src" in
    http://*|https://*|ftp://*) ./revert acquire-game-data --url "$src" || die "download/acquire failed";;
    *) ./revert acquire-game-data --folder "$src" || die "acquire failed";;
  esac
fi

# ── 7. build the edition ──────────────────────────────────────────────────────
step "Building your edition"
./revert build || die "build failed — see the messages above."

# ── 8. Steam Deck controller calibration (needs the built game) ───────────────
if is_deck; then
  step "Detecting your controller"
  ./revert calibrate-controller || warn "controller calibration hiccuped — re-run later: ./revert calibrate-controller"
fi

# ── done ──────────────────────────────────────────────────────────────────────
printf '\n%s✓ All done!%s\n' "$g" "$o"
if is_deck; then
  info "Switch to Gaming Mode and launch \"Tony Hawk's Underground 2 (VV Edition)\" from your library."
else
  info "Play it:  cd $DEST && ./revert run qol      (or ./revert gui for a clickable menu)"
fi
