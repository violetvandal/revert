#!/usr/bin/env bash
#
# revert-setup.sh — one-time system + Wine setup for THUG2: Violet Vandal Edition.
# Supersedes the system-setup half of the legacy install.sh, retargeted to the
# current architecture: GE-Proton wine + two prefixes (main + online), three lanes.
# Drops the abandoned Clownjob'd dual-profile.
#
#   revert-setup.sh [--no-packages] [--online]
#     --no-packages   skip the `sudo dnf` step (deps already installed)
#     --online        also prepare the THUG Pro (online) prefix
#
# Idempotent: existing prefixes are reused, not recreated. Wine work uses GE-Proton
# (system wine on Fedora is wow64-only and can't do the win32 prefix THUG2 needs).
# Installing GE-Proton itself (via Lutris / ProtonUp-Qt) is a prerequisite this
# script checks for but does not automate.
#
set -euo pipefail

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

log()  { printf '\033[1;34m[setup]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[setup:warn]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m[setup:error]\033[0m %s\n' "$*" >&2; exit 1; }
# Run a command as root. When SUDO_ASKPASS is set (the GUI installer points it at a
# helper that supplies the password), use `sudo -A` so no terminal prompt is needed;
# otherwise fall back to a normal interactive sudo.
ask_sudo() {
  log "running as root: $*"
  if [[ -n "${SUDO_ASKPASS:-}" ]]; then sudo -A "$@"; else sudo "$@"; fi
}

# Steam Deck? (Game-Mode env, or DMI board name Jupiter=LCD / Galileo=OLED)
is_steam_deck() {
  [[ "${SteamDeck:-0}" == "1" ]] && return 0
  local pn=/sys/devices/virtual/dmi/id/product_name
  [[ -r "$pn" ]] && grep -qiE 'jupiter|galileo' "$pn" && return 0
  return 1
}
IS_DECK=0; if is_steam_deck; then IS_DECK=1; fi

# Kron4ek wine 11.11 — current SteamOS glibc (2.41) crashes every 32-bit app under
# the 2023 wine-ge-8-26; a current wine is the one fix. (memory project_steamdeck_lane)
WINE_DECK_URL="https://github.com/Kron4ek/Wine-Builds/releases/download/11.11/wine-11.11-staging-amd64.tar.xz"

DO_PACKAGES=1; DO_ONLINE=0; SKIP_BASE=0
for a in "$@"; do
  case "$a" in
    --no-packages) DO_PACKAGES=0;;
    --online)      DO_ONLINE=1;;
    # Just the online (THUG Pro) lane: skip packages + the main prefix. Used by the GUI's
    # "install THUG Pro" button so a one-off online install doesn't re-run the whole setup.
    --online-only) DO_ONLINE=1; SKIP_BASE=1;;
  esac
done

GE_WINE="$GE_DIR/bin/wine"

# On OSTree-based distros (Bazzite, Silverblue, Kinoite) the system SDL2 is 64-bit
# only, but Wine's 32-bit winebus.sys needs libSDL2-2.0.so.0 (i686) to enumerate
# gamepads. Symlink ONLY libSDL2 into a private dir — the full Steam runtime ships
# libvulkan.so.1 which shadows the system Vulkan ICD and breaks DXVK.
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

# ---- system packages ----------------------------------------------------------
# `revert uninstall --purge` offers to remove the packages setup installed. It may only
# remove the ones that were NOT already on the machine — pulling out a 32-bit SDL2 or
# glibc that predates Revert would break unrelated software. So record exactly what this
# run added, one "<manager> <package>" line per package, and never guess later.
PKG_MANIFEST="${REVERT_ROOT}/.revert-packages"

record_packages() {  # $1 = manager, rest = packages this run actually installed
  local mgr="$1"; shift
  (( $# )) || return 0
  local p
  for p in "$@"; do
    grep -qxF "$mgr $p" "$PKG_MANIFEST" 2>/dev/null || printf '%s %s\n' "$mgr" "$p" >> "$PKG_MANIFEST"
  done
  log "  recorded $# newly-installed package(s) for a future uninstall --purge"
}

# Steam Deck: the 32-bit X libs THUG2's win32 wine needs (without them the game
# can't create a window: nodrv_CreateWindow). The pad bridge is stdlib-only and
# uinput is granted by ACL, so no python3-evdev needed on Deck.
install_packages_deck() {
  local libs=(lib32-libxrender lib32-libxcursor lib32-libxi lib32-libxrandr lib32-libxcomposite lib32-libxkbcommon)
  if pacman -Qq "${libs[@]}" >/dev/null 2>&1; then
    log "32-bit X libs already present"
    return 0
  fi
  local new=() p
  for p in "${libs[@]}"; do pacman -Qq "$p" >/dev/null 2>&1 || new+=("$p"); done
  log "installing 32-bit X libs (sudo; toggles SteamOS read-only + inits the pacman keyring)"
  ask_sudo steamos-readonly disable || warn "steamos-readonly disable failed"
  ask_sudo pacman-key --init        >/dev/null 2>&1 || true
  ask_sudo pacman-key --populate    >/dev/null 2>&1 || true   # keyring is empty on a fresh Deck
  if ask_sudo pacman -Sy --needed --noconfirm "${libs[@]}"; then
    record_packages pacman "${new[@]}"
  else
    warn "pacman install failed — install these manually: ${libs[*]}"
  fi
}

install_packages() {
  if (( IS_DECK )); then install_packages_deck; return $?; fi
  command -v dnf >/dev/null || { warn "non-Fedora system: install equivalents of winetricks p7zip msitools cabextract python3-evdev yourself"; return 0; }
  local pkgs=(winetricks p7zip p7zip-plugins msitools cabextract python3-evdev)
  local new=() p
  for p in "${pkgs[@]}"; do rpm -q "$p" >/dev/null 2>&1 || new+=("$p"); done
  log "installing system packages (sudo)"
  ask_sudo dnf install -y "${pkgs[@]}"
  record_packages dnf "${new[@]}"
}

# ---- wine runtime presence ----------------------------------------------------
check_ge() {
  if [[ -x "$GE_WINE" ]]; then
    log "wine runtime: $GE_DIR"
    return 0
  fi
  if (( IS_DECK )); then
    # Install Kron4ek wine 11.11 (GE_DIR should point at .../wine-11.11-staging-amd64).
    # Prefer the bundled archive shipped by sync-to-deck.sh (offline / no URL-rot);
    # fall back to downloading from WINE_DECK_URL.
    local parent; parent="$(dirname "$GE_DIR")"; mkdir -p "$parent"
    local arc="${WINE_DECK_ARCHIVE:-$REVERT_ROOT/tools/wine-11.11-staging-amd64.tar.xz}"
    if [[ -f "$arc" ]]; then
      log "installing bundled wine 11.11 ($(basename "$arc"))"
      tar xJf "$arc" -C "$parent" || err "wine extract failed ($arc)"
    else
      log "wine missing + no bundled archive — downloading Kron4ek wine 11.11"
      command -v curl >/dev/null || err "curl needed to download wine"
      local tmp; tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' RETURN
      curl -fL --retry 3 -o "$tmp/wine.tar.xz" "$WINE_DECK_URL" || err "wine download failed ($WINE_DECK_URL)"
      tar xJf "$tmp/wine.tar.xz" -C "$parent" || err "wine extract failed"
    fi
    [[ -x "$GE_WINE" ]] || err "wine still missing at $GE_DIR after install — check that GE_DIR matches the extracted dir name"
    log "wine 11.11 installed: $GE_DIR"
    return 0
  fi
  err "GE-Proton wine not found at $GE_DIR
  Install a GE-Proton/wine-ge runner (via Lutris or ProtonUp-Qt) and point GE_DIR
  in revert.conf at it. System Fedora wine is wow64-only and cannot host THUG2's
  win32 prefix."
}

# ---- a win32 prefix -----------------------------------------------------------
init_prefix() {  # $1 = prefix path
  local pfx="$1"
  if [[ -d "$pfx/drive_c" ]]; then log "prefix exists: $pfx (reuse)"; return 0; fi
  log "creating win32 prefix: $pfx"
  # mscoree/mshtml=d so wineboot never blocks on the Mono/Gecko prompt (Kron4ek
  # doesn't bundle them; THUG2 needs neither — the online prefix gets Mono later).
  WINEARCH=win32 WINEPREFIX="$pfx" WINEDLLOVERRIDES="mscoree=d;mshtml=d" WINEDEBUG=-all \
    "$GE_WINE" wineboot >/dev/null 2>&1 || err "wineboot failed for $pfx"
}

# ---- wine virtual desktop (Deck: avoids fullscreen mode-change black screens) --
set_virtual_desktop() {  # $1 = prefix, $2 = WxH (e.g. 1280x800)
  WINEPREFIX="$1" WINEDEBUG=-all "$GE_WINE" reg add \
    "HKCU\\Software\\Wine\\Explorer" /v Desktop /d Default /f >/dev/null 2>&1 || true
  WINEPREFIX="$1" WINEDEBUG=-all "$GE_WINE" reg add \
    "HKCU\\Software\\Wine\\Explorer\\Desktops" /v Default /d "$2" /f >/dev/null 2>&1 \
    && log "  virtual desktop $2 set" || warn "  virtual desktop set failed"
}

# ---- DXVK into a prefix --------------------------------------------------------
install_dxvk() {  # $1 = prefix path
  local pfx="$1" archive="${REVERT_ROOT}/tools/dxvk-${DXVK_VERSION}.tar.gz"
  local work; work="$(mktemp -d)"; trap 'rm -rf "$work"' RETURN
  if [[ ! -f "$archive" ]]; then
    log "downloading DXVK $DXVK_VERSION"
    curl -fsSL -o "$archive" \
      "https://github.com/doitsujin/dxvk/releases/download/v${DXVK_VERSION}/dxvk-${DXVK_VERSION}.tar.gz" \
      || err "DXVK download failed"
  fi
  tar xzf "$archive" -C "$work"
  cp "$work/dxvk-${DXVK_VERSION}/x32/"*.dll "$pfx/drive_c/windows/system32/" || err "DXVK copy failed"
  for dll in d3d9 d3d10core d3d11 dxgi; do
    WINEPREFIX="$pfx" WINEDEBUG=-all "$GE_WINE" reg add \
      "HKCU\\Software\\Wine\\DllOverrides" /v "$dll" /d native /f >/dev/null 2>&1 || true
  done
  log "  DXVK $DXVK_VERSION installed into $(basename "$pfx")"
}

# ---- winetricks components (main prefix) --------------------------------------
# NOTE: do NOT install `dinput8` here. winetricks' dinput8 drops the *native* Microsoft
# dinput8.dll (override=native), and native dinput8 under Wine cannot enumerate winebus/
# SDL game controllers — THUG2 (a DirectInput game) then sees NO pad at all. Wine's
# builtin dinput8 enumerates the pad correctly (as "Controller (XBOX 360 For Windows)",
# guidInstance matching the saved pad0). set_dinput8_builtin() enforces builtin below.
install_winetricks_components() {  # $1 = prefix
  command -v winetricks >/dev/null || { warn "winetricks absent — skipping d3dx9"; return 0; }
  log "winetricks: d3dx9 sound=pulse"
  WINEPREFIX="$1" WINE="$GE_WINE" WINEDEBUG=-all winetricks -q d3dx9 sound=pulse || warn "winetricks step had issues"
}

# ---- WSFix winmm override (main prefix) ---------------------------------------
set_winmm_override() {  # $1 = prefix
  WINEPREFIX="$1" WINEDEBUG=-all "$GE_WINE" reg add \
    "HKCU\\Software\\Wine\\DllOverrides" /v winmm /d "native,builtin" /f >/dev/null 2>&1 \
    && log "  winmm=native,builtin set (WSFix loader)" || warn "  winmm override failed"
}

# ---- dinput8 = builtin (main prefix) -----------------------------------------
# Force Wine's builtin dinput8 so DirectInput enumerates the controller. Clears any
# stale native override + `*dinput8` key a prior winetricks run may have left behind.
set_dinput8_builtin() {  # $1 = prefix
  WINEPREFIX="$1" WINEDEBUG=-all "$GE_WINE" reg delete \
    "HKCU\\Software\\Wine\\DllOverrides" /v "*dinput8" /f >/dev/null 2>&1 || true
  WINEPREFIX="$1" WINEDEBUG=-all "$GE_WINE" reg add \
    "HKCU\\Software\\Wine\\DllOverrides" /v dinput8 /d "builtin" /f >/dev/null 2>&1 \
    && log "  dinput8=builtin set (DirectInput controller enumeration)" || warn "  dinput8 override failed"
}

# ---- Steam Deck controller (main prefix) -------------------------------------
# Steam Input's emulated Xbox pad intermittently stalls Wine's DInput state mid-game.
# The pad-mirror bridge fixes it: it creates a STABLE virtual analog pad ("Violet Vandal
# Pad") and continuously mirrors Steam's pad into it, and THUG2 binds to OURS — so the
# game never sees Steam's flaky pad. So: import the gp0_/k0_ map, start the bridge (which
# creates the virtual pad regardless of whether Steam is up), then set pad0 to OUR pad's
# guidInstance (detected live if the probe works, else the known deterministic GUID).
# memory: project_steamdeck_controller
setup_controller_deck() {  # $1 = prefix
  local pfx="$1" st="$CONTROLS_DIR/thug2-settings-deck.reg"
  if [[ -f "$st" ]]; then
    WINEPREFIX="$pfx" WINEDEBUG=-all "$GE_WINE" reg import "$st" >/dev/null 2>&1 \
      && log "  Deck controller binding imported (gp0_/k0_)" || warn "  Deck binding import failed"
  else
    warn "  $st missing — controller will not be bound"
  fi
  local bpid="" guid="" i
  local re='^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$'
  # Detect against a FRESH wineserver with exactly ONE virtual pad — Wine's instance GUID
  # for our pad depends on how many it has seen (a 2nd concurrent/sequential one becomes
  # #2 with a different GUID). The real game launch starts a fresh wineserver + one vpad
  # (so it gets #1); match that here by clearing the game, leftover bridges, AND the
  # wineserver this setup run has been using, before probing.
  kill -9 $(pgrep -x THUG2.exe) 2>/dev/null || true
  pkill -f thug2-pad-mirror.py 2>/dev/null || true; sleep 1
  "$GE_DIR/bin/wineserver" -k 2>/dev/null || true; pkill -9 -x services.exe 2>/dev/null || true; sleep 1
  if [[ -f "$PAD_BRIDGE" ]] && command -v python3 >/dev/null; then
    python3 "$PAD_BRIDGE" >/dev/null 2>&1 & bpid=$!
    for i in $(seq 1 30); do grep -q "Violet Vandal Pad" /proc/bus/input/devices 2>/dev/null && break; sleep 0.2; done
  else
    warn "  pad-mirror bridge missing ($PAD_BRIDGE) — cannot set pad0"
  fi
  [[ -n "$bpid" ]] && kill "$bpid" 2>/dev/null || true
  # pad0 (which virtual pad THUG2 binds to) is NOT set here. Wine 11.11 gives the pad a
  # PER-PREFIX DInput instance GUID that can only be read from the game's own +dinput trace
  # (the standalone probe hangs on Wine 11.11), and the game doesn't exist yet at setup time
  # (build runs later). `revert calibrate-controller` sets pad0 after the build — the
  # bootstrap runs it automatically; manual installs run it once after `revert build`.
  log "  controller bindings imported — pad0 set by 'revert calibrate-controller' after the build"
  [[ -w /dev/uinput ]] || warn "  /dev/uinput not writable — the pad-mirror bridge needs it"
  # leave the prefix's wineserver torn down so the first game launch starts clean
  "$GE_DIR/bin/wineserver" -k 2>/dev/null || true
}

# ---- Steam library shortcut (Deck) -------------------------------------------
setup_steam_shortcut_deck() {
  local play="$REVERT_ROOT/play-qol.sh"
  if [[ ! -f "$play" ]]; then
    cat > "$play" <<EOF
#!/usr/bin/env bash
cd "$REVERT_ROOT"
exec ./revert run qol >"$REVERT_ROOT/deck-run.log" 2>&1
EOF
    chmod +x "$play"; log "  created launcher $play"
  fi
  local tool="$REVERT_ROOT/tools/deck/add-steam-shortcut.py"
  local art="$REVERT_ROOT/tools/deck/art"
  local name="Tony Hawk's Underground 2 (VV Edition)"
  local oldname="THUG2: Violet Vandal Edition"
  local icon="$REVERT_ROOT/game-playable-us/THUG2.ico"
  [[ -f "$icon" ]] || icon="$art/icon.png"; [[ -f "$icon" ]] || icon=""
  [[ -f "$tool" ]] || { warn "  shortcut tool missing ($tool)"; return 0; }

  # shortcuts.vdf must be written while Steam is NOT running (Steam overwrites it on
  # exit). But Steam is also the Deck's on-screen keyboard, so we can't ask the user to
  # close it before running setup. Instead — now that the password-needing steps are
  # done — shut Steam down cleanly ourselves, write the shortcut, and relaunch it.
  # (Safe: setup is launched from Konsole/the .desktop, NOT through Steam, so closing
  # Steam doesn't kill us.)
  local reopen=0 i
  if pgrep -x steam >/dev/null; then
    if pgrep -x gamescope >/dev/null; then     # Gaming Mode IS Steam — don't kill it
      warn "  in Gaming Mode — run setup in Desktop Mode for the auto-shortcut, or add via Add a Non-Steam Game -> $play"
      return 0
    fi
    log "  registering Steam shortcut (Steam will briefly close + reopen)…"
    command -v steam >/dev/null && steam -shutdown >/dev/null 2>&1 || true
    pkill -TERM -x steam 2>/dev/null || true
    for i in $(seq 1 30); do pgrep -x steam >/dev/null || break; sleep 1; done
    if pgrep -x steam >/dev/null; then
      warn "  Steam wouldn't close — add it manually: Add a Non-Steam Game -> $play"
      return 0
    fi
    reopen=1
  fi
  # drop the old-named shortcut from earlier installs (a rename = new appid, so the old
  # entry would otherwise linger as a duplicate). Steam is already closed at this point.
  python3 "$tool" --name "$oldname" --remove >/dev/null 2>&1 || true
  if python3 "$tool" --name "$name" --exe "$play" \
       --startdir "$REVERT_ROOT" --icon "$icon" --art "$art"; then
    log "  Steam shortcut + tile art registered (launch from your library in Gaming Mode)"
  else
    warn "  shortcut add failed — add manually: Add a Non-Steam Game -> $play"
  fi
  # Reopen Steam only for a manual Desktop-Mode setup (restore the user's session). Under
  # the GUI installer (REVERT_DRIVEN=1) leave it closed: reopening pops Steam in front of
  # the installer, and the new shortcut is picked up anyway when they enter Gaming Mode.
  if (( reopen )); then
    if [[ "${REVERT_DRIVEN:-0}" == 1 ]]; then
      log "  Steam left closed — your new shortcut appears when you switch to Gaming Mode"
    else
      log "  reopening Steam…"
      setsid steam >/dev/null 2>&1 < /dev/null &
    fi
  fi
}

# ---- native PS2-style controller (main prefix) -------------------------------
setup_controller() {  # $1 = prefix
  if (( IS_DECK )); then setup_controller_deck "$1"; return $?; fi
  local ov="$CONTROLS_DIR/thug2-joystick-override.reg" st="$CONTROLS_DIR/thug2-settings.reg"
  if [[ -f "$ov" && -f "$st" ]]; then
    WINEPREFIX="$1" WINEDEBUG=-all "$GE_WINE" reg import "$ov" >/dev/null 2>&1 || warn "  override import failed"
    WINEPREFIX="$1" WINEDEBUG=-all "$GE_WINE" reg import "$st" >/dev/null 2>&1 || warn "  settings import failed"
    log "  controller bindings + trigger-split override imported"
  else
    warn "  controller .reg files missing in $CONTROLS_DIR"
  fi
  # pad0 (which physical gamepad THUG2 binds to) is NOT set here — `revert run` detects
  # the live GUID via the padfix hook before each launch, so it always reflects the actual
  # SDL-assigned guidInstance even across Wine updates or prefix rebuilds.
  [[ -f "$TRIGGER_BRIDGE" ]] || warn "  trigger-bridge script missing ($TRIGGER_BRIDGE)"
  if [[ ! -w /dev/uinput ]]; then
    warn "  /dev/uinput not writable — the L2/R2 trigger bridge needs it. To grant access:
    sudo groupadd -f input && sudo usermod -aG input \"$USER\"
    echo 'KERNEL==\"uinput\", GROUP=\"input\", MODE=\"0660\"' | sudo tee /etc/udev/rules.d/99-uinput.rules
    sudo udevadm control --reload-rules && sudo udevadm trigger   (then re-login)"
  fi
}

# ---- online lane: Mono + THUG Pro installer (Wine) ----------------------------
# THUG Pro's .NET launcher needs Mono in the online prefix. GE-Proton bundles a
# wine-mono; copy it into the prefix's mono dir (msiexec install fails on path
# translation, but the plain copy works — see memory project_thugpro_profile).
setup_mono_online() {  # $1 = prefix
  local pfx="$1" dst="$1/drive_c/windows/mono/mono-2.0"
  [[ -d "$dst" ]] && { log "  Mono already present in the online prefix"; return 0; }
  local src; src="$(ls -d "$GE_DIR"/share/wine/mono/wine-mono-* 2>/dev/null | sort -V | tail -1)"
  if [[ -z "$src" || ! -d "$src" ]]; then
    warn "  GE-Proton bundled Mono not found under $GE_DIR/share/wine/mono — THUG Pro's launcher may not start"
    return 0
  fi
  mkdir -p "$(dirname "$dst")"
  cp -a "$src" "$dst" && log "  Mono installed into the online prefix ($(basename "$src"))" \
    || warn "  Mono copy failed"
}

# Acquire THUG Pro's official installer (BYO $THUGPRO_SETUP wins, else download
# $THUGPRO_SETUP_URL) and run it under Wine in the online prefix. Skips cleanly if THUG Pro
# is already installed, so it is safe to re-run. THUG Pro's own installer then asks for a
# clean THUG2 folder and pulls the full build itself.
acquire_launch_thugpro() {  # $1 = prefix
  local pfx="$1"
  local launcher="$pfx/drive_c/users/$USER/AppData/Local/THUG Pro/THUGProLauncher.exe"
  if [[ -f "$launcher" ]]; then
    log "  THUG Pro already installed in the online prefix — skipping installer"
    return 0
  fi
  local exe="${THUGPRO_SETUP:-}"
  if [[ -n "$exe" && -f "$exe" ]]; then
    log "  using bundled THUG Pro installer: $exe"
  else
    local url="${THUGPRO_SETUP_URL:-}"
    [[ -n "$url" ]] || { warn "  no THUG Pro installer (THUGPRO_SETUP) and no THUGPRO_SETUP_URL — install it manually"; return 0; }
    command -v curl >/dev/null || { warn "  curl needed to download the THUG Pro installer"; return 0; }
    exe="$REVERT_ROOT/.revert-thugpro-setup.exe"
    log "  downloading the THUG Pro installer from $url"
    curl -fL --retry 3 -o "$exe" "$url" || { warn "  THUG Pro installer download failed ($url)"; return 0; }
  fi
  log "  launching the THUG Pro installer under Wine — point it at a CLEAN THUG2 folder (not the modded edition)"
  WINEPREFIX="$pfx" WINEDLLOVERRIDES="mscoree=b" WINEDEBUG=-all "$GE_WINE" "$exe" \
    || warn "  the THUG Pro installer exited non-zero (if it did not appear, check that Mono installed above)"
  [[ -f "$launcher" ]] && log "  THUG Pro installed. Play: revert run online" \
    || note "  once THUG Pro finishes installing, launch it with: revert run online"
}

# ---- main ---------------------------------------------------------------------
(( SKIP_BASE )) || { (( DO_PACKAGES )) && install_packages || log "skipping package install (--no-packages)"; }
check_ge

if (( ! SKIP_BASE )); then
  log "== main prefix (Vanilla + QOL-Modded) =="
  init_prefix "$PREFIX_MAIN"
  # On Bazzite/OSTree, z: -> / (composefs, 0 bytes free). Add d: so the game runs
  # on a drive backed by the real data partition and GetDiskFreeSpaceEx returns real stats.
  ln -sfn "$REVERT_ROOT" "${PREFIX_MAIN}/dosdevices/d:"
  (( IS_DECK )) && set_virtual_desktop "$PREFIX_MAIN" 1280x800
  install_dxvk "$PREFIX_MAIN"
  install_winetricks_components "$PREFIX_MAIN"
  set_winmm_override "$PREFIX_MAIN"
  set_dinput8_builtin "$PREFIX_MAIN"
  setup_controller "$PREFIX_MAIN"
  (( IS_DECK )) && setup_steam_shortcut_deck
fi

if (( DO_ONLINE )); then
  log "== online prefix (THUG Pro) =="
  init_prefix "$PREFIX_ONLINE"
  install_dxvk "$PREFIX_ONLINE"
  setup_mono_online "$PREFIX_ONLINE"
  acquire_launch_thugpro "$PREFIX_ONLINE"
fi

# App-menu launcher for the click-to-play GUI (no terminal needed afterwards).
if (( ! SKIP_BASE )) && [[ -x "${REVERT_ROOT}/revert" ]]; then
  "${REVERT_ROOT}/revert" install-desktop || warn "desktop launcher install skipped (non-fatal)"
fi

if (( SKIP_BASE )); then
  log "online setup done."
else
  log "setup complete. Next: revert acquire-game-data  then  revert build  then  revert run qol"
fi
