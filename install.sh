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
is_mac()  { [[ "$(uname -s)" == "Darwin" ]]; }
# Apple Silicon vs Intel. The lane is proven on Apple Silicon; Intel is expected to work
# (the DXVK patch targets Metal, which has no geometry shaders on ANY Mac GPU, and Intel
# skips Rosetta entirely) but is untested, so we say so rather than pretend either way.
is_apple_silicon() { [[ "$(uname -m)" == "arm64" ]]; }

# has_password — authoritative check: does the current account have a usable password?
# `passwd -S` field 2 is P (usable) / NP (none) / L (locked). Reading the real state
# beats trusting an exit code from whatever tool we used to set it.
has_password() { [[ "$(passwd -S 2>/dev/null | awk 'NR==1{print $2}')" == "P" ]]; }

# set_password_noninteractive <pw> — set the CURRENT user's password without a keyboard.
# `passwd` reads its controlling terminal (getpass), not a pipe, so we drive it through a
# PTY. A fresh SteamOS 'deck' account has no password and `passwd` asks only for the new
# one twice (the documented Deck flow). We try util-linux `script`, then a real Python
# PTY, and judge success by has_password — NOT by the driver's exit code (script -e can
# report nonzero even when the change took). Captures passwd's message for diagnostics.
VV_PASSWD_MSG=""
set_password_noninteractive() {
  local pw="$1" log; log="$(mktemp)"
  # Attempt 1: util-linux `script` provides the PTY; feed the new password twice.
  if command -v script >/dev/null; then
    printf '%s\n%s\n' "$pw" "$pw" | script -qec passwd /dev/null >"$log" 2>&1 || true
    has_password && { rm -f "$log"; return 0; }
  fi
  # Attempt 2: a real PTY via Python (present on SteamOS); react to each prompt.
  if command -v python3 >/dev/null; then
    REVERT_PW="$pw" python3 - >>"$log" 2>&1 <<'PY' || true
import os, pty, time, select, sys
pw = (os.environ["REVERT_PW"] + "\n").encode()
pid, fd = pty.fork()
if pid == 0:
    os.execvp("passwd", ["passwd"])
writes, buf, end = 0, b"", time.time() + 20
while time.time() < end:
    try:
        r, _, _ = select.select([fd], [], [], 0.5)
    except OSError:
        break
    if r:
        try:
            d = os.read(fd, 4096)
        except OSError:
            break
        if not d:
            break
        buf += d
    # Answer each "…:" prompt (New / Retype) with the password, at most twice.
    if writes < 2 and buf.rstrip().endswith(b":"):
        try:
            os.write(fd, pw)
        except OSError:
            break
        writes += 1
        buf = b""
try:
    os.waitpid(pid, 0)
except OSError:
    pass
PY
    has_password && { rm -f "$log"; return 0; }
  fi
  # Failed — surface passwd's real message (echo is off, so no password leaks into it).
  VV_PASSWD_MSG="$(grep -aiE 'bad|error|fail|token|short|simple|dictionary|palindrome|no tty|not found|denied' "$log" \
    | grep -aviE 'new password|retype|current password' | sort -u | head -5)"
  rm -f "$log"
  return 1
}

printf '%s\n' "${p}"
cat <<'BANNER'
  ┌──────────────────────────────────────────────────┐
  │   THUG2: Violet Vandal Edition — installer         │
  │   Tony Hawk's Underground 2, modernized            │
  └──────────────────────────────────────────────────┘
BANNER
printf '%s' "${o}"
if is_mac; then
  if is_apple_silicon; then
    info "macOS on Apple Silicon detected."
  else
    info "macOS on an Intel Mac detected — supported (native x86, no Rosetta)."
  fi
elif is_deck; then
  info "Steam Deck detected."
else
  info "Running on a Linux desktop."
fi

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
if ! command -v git >/dev/null; then
  is_mac && die "git isn't installed. Run 'xcode-select --install' to get Apple's command
  line tools (which include git), then re-run this."
  die "git isn't installed. On the Steam Deck it's built in; otherwise install 'git' with \
your package manager, then re-run this."
fi
# On macOS /usr/bin/git is a STUB that only works once Apple's Command Line Tools are
# installed. `command -v git` passes for the stub, but actually running git then errors
# ("No developer tools were found") and pops a GUI dialog — which on a genuinely fresh Mac
# used to surface only as a misleading "clone failed (network?)". Detect it up front (via
# xcode-select, which never triggers the dialog) and kick off Apple's installer.
if is_mac && ! xcode-select -p >/dev/null 2>&1; then
  info "Apple's Command Line Tools (which include a working git) aren't installed — opening Apple's installer."
  xcode-select --install >/dev/null 2>&1 || true
  die "Click Install in the macOS \"Command Line Developer Tools\" dialog, let it finish, then run this installer again."
fi
ok "git present"

# ── 2. account password (fresh SteamOS 'deck' user has none; setup needs sudo) ─
# macOS accounts always have a password, and `passwd -S` means something different there,
# so this whole step is Linux-only.
if is_mac; then
  pwstat="P"
else
  step "Checking your account password"
  pwstat="$(passwd -S 2>/dev/null | awk 'NR==1{print $2}')"
fi
if [[ "$pwstat" == "NP" || "$pwstat" == "L" ]]; then
  if [[ "$DRIVEN" == 1 ]]; then
    [[ -n "${REVERT_PASSWORD:-}" ]] || die "no password provided (setup needs one to install a few system libraries)."
    info "Setting your account password (needed for the one system step)."
    if set_password_noninteractive "$REVERT_PASSWORD"; then
      ok "password set"
    else
      [[ -n "$VV_PASSWD_MSG" ]] && { warn "the system rejected it:"; printf '%s\n' "$VV_PASSWD_MSG" | sed 's/^/      /'; }
      die "couldn't set your account password automatically.$([[ -n "$VV_PASSWD_MSG" ]] && echo ' See the reason above.') \
Most often the password is too short/simple — pick a longer one (8+ chars, not all digits) and press Install again. \
Or set it yourself: open Konsole, run 'passwd' (use the SAME password you typed here), then press Install again."
    fi
  else
    warn "You don't have a password set yet — the setup step needs one (it installs a"
    info "few system libraries). Let's set it now. Type a new password twice; nothing"
    info "shows on screen as you type. Remember it — you'll use it for the setup step."
    passwd </dev/tty || die "couldn't set a password. Run 'passwd' yourself, then re-run this installer."
    ok "password set"
  fi
else
  is_mac || ok "password is set"
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
  goos=linux; is_mac && goos=darwin
  tgz="${GO_VER}.${goos}-${arch}.tar.gz"
  mkdir -p "$HOME/.local"
  if command -v curl >/dev/null; then curl -fL "https://go.dev/dl/$tgz" -o "/tmp/$tgz" || die "Go download failed"
  else wget -O "/tmp/$tgz" "https://go.dev/dl/$tgz" || die "Go download failed"; fi
  rm -rf "$HOME/.local/go"
  tar -C "$HOME/.local" -xzf "/tmp/$tgz" || die "Go extract failed"; rm -f "/tmp/$tgz"
  export PATH="$HOME/.local/go/bin:$PATH"
  command -v go >/dev/null && ok "Go installed ($(go version | awk '{print $3}'))" || die "Go install failed"
  # Mark that WE installed this Go, so `revert uninstall --purge` can remove it and a
  # user's pre-existing Go is never touched. The marker moves into the clone below.
  WE_INSTALLED_GO=1
fi

# ── 4. fetch the toolkit ──────────────────────────────────────────────────────
step "Fetching the Revert toolkit"
if [[ -d "$DEST/.git" ]]; then
  info "already have it at $DEST — updating"
  git -C "$DEST" pull --recurse-submodules --ff-only || warn "update skipped (local changes?)"
  git -C "$DEST" submodule update --init --recursive || true
  ok "up to date: $DEST"
else
  # A non-empty destination that isn't a Revert clone. This is easy to hit on a Mac: the
  # filesystem is case-INsensitive, so a user who keeps their game copy at ~/THUG2 already
  # occupies ~/thug2. git's own error for this is cryptic, so say what's actually wrong.
  if [[ -d "$DEST" ]] && [[ -n "$(ls -A "$DEST" 2>/dev/null)" ]]; then
    die "$DEST already exists and isn't a Revert install.
  (On a Mac, note that ~/THUG2 and ~/thug2 are the SAME folder — the filesystem ignores case.)
  Move it aside, or install somewhere else:  REVERT_DIR=~/my-thug2 bash install.sh"
  fi
  git clone --recursive "$REPO_URL" "$DEST" || die "clone failed (network?)"
  ok "installed to $DEST"
fi
cd "$DEST" || die "cannot enter $DEST"

# Leave a breadcrumb the uninstaller reads: only a Go WE fetched into ~/.local/go is safe
# for `revert uninstall --purge` to remove. A Go the user already had stays put.
if [[ "${WE_INSTALLED_GO:-0}" == 1 ]]; then
  printf '%s\n' "$HOME/.local/go" > "$DEST/.revert-installed-go" 2>/dev/null || true
fi

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

# ── 5. system setup ───────────────────────────────────────────────────────────
if is_mac; then
  step "System setup — Wine, patched DXVK, controller, app bundles"
  info "This downloads Wine (~176MB, checksum-verified) into the toolkit. No Homebrew and"
  info "no admin needed — nothing is installed system-wide."
else
  step "System setup — Wine, controller, Steam shortcut"
  info "This is the step that needs your password (to install a few 32-bit libraries)."
fi
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
elif is_mac; then
  info "Play it:  open \"THUG2 Violet Vandal Edition\" from ~/Applications or Spotlight"
  info "     or:  cd $DEST && ./revert run qol"
  info "Controller: pair an Xbox pad in XInput mode (macOS only exposes Microsoft-VID pads to Wine)."
else
  info "Play it:  cd $DEST && ./revert run qol      (or ./revert gui for a clickable menu)"
fi
