#!/usr/bin/env bash
#
# THUG2 Ultimate Edition Installer
# Tony Hawk's Underground 2 — modernized for Linux Wine
#
# Builds two parallel game folders sharing one Data tree via symlink:
#   <install-dir>/shared/Data/                — game data (extracted from MSI)
#   <install-dir>/vanilla/                    — THUG2.exe + WidescreenFixesPack (best on ultrawide; native analog controller)
#   <install-dir>/clownjobd/                  — THUGTWO.exe + Clownjob'd (PS2 controls, OpenSpy, etc.)
#   <install-dir>/launch-vanilla.sh           — launcher for the vanilla profile
#   <install-dir>/launch-clownjobd.sh         — launcher for the Clownjob'd profile
#   <install-dir>/configure-controls.sh       — THUG2 config launcher (bind keyboard/gamepad)
#
# Controller note: THUG2 has NATIVE DirectInput gamepad support (analog sticks), read
# from the registry. install.sh auto-imports a PS2-style Xbox-360 map + a trigger-split
# Override (setup_controller); the vanilla launcher also starts a small Python bridge that
# remaps the triggers/bumpers (LT/RT=Nollie/Switch, LB/RB=spin, LB+RB=walk) — see the
# setup_controller comment for why. To re-bind for a different pad, run configure-controls.sh.
# DEPENDENCY for the bridge: python3 + python-evdev (Fedora: python3-evdev).
# The vanilla profile loads WidescreenFixesPack via a winmm.dll proxy (NOT dinput8.dll)
# specifically so dinput8 stays Wine-native and the game can enumerate the controller.
# (Clownjob'd's own XInput support does not work under Wine — its in-game hooks fail —
# so the native DInput path + bridge is the way controllers work here.)
#
# Usage:
#   ./install.sh --iso <path-to-thug2.iso> --clownjobd-zip <path-to-thug2_clownjobd.zip>
#
# Inputs required:
#   --iso             Path to the THUG2 PC ISO (EU or US — only the MSI inside is used)
#   --clownjobd-zip   Path to THUG2_ClownJob'd_v1.3.zip (contains THUGTWO.exe + DLL + ini)
#
# Env var overrides:
#   THUG2_INSTALL_DIR  default: $HOME/Games/THUG2
#   THUG2_WINE_PREFIX  default: $HOME/.wine-thug2
#   THUG2_WORK_DIR     default: $HOME/.cache/thug2-install
#   THUG2_RES_W        default: detected from xrandr (else 1920)
#   THUG2_RES_H        default: detected from xrandr (else 1080)
#
# Audience: Linux users who own the game and want a one-command modernized install.
# Tested on Fedora 43 with Wine 11 Staging. Other distros: adapt install_packages().

set -euo pipefail

# ---------- Configuration ----------

INSTALL_DIR="${THUG2_INSTALL_DIR:-$HOME/Games/THUG2}"
WINE_PREFIX="${THUG2_WINE_PREFIX:-$HOME/.wine-thug2}"
WORK_DIR="${THUG2_WORK_DIR:-$HOME/.cache/thug2-install}"
# Repo dir (for shipped artifacts: trigger-bridge + controller .reg files)
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

ISO_PATH=""
CLOWNJOBD_ZIP=""
XBOX_HQ_PATH=""   # optional, downloads from archive.org if not given
HQ_TEXTURES_ZIP="" # optional; if provided, applies Zure's HQ Classic Level Textures

# Pinned versions of community tools (bump deliberately, not casually)
DXVK_VERSION="2.7.1"
WSFIX_TAG="thug2"
XBOX_HQ_URL="https://archive.org/download/thug-2-hq-xbox-video-and-audio-pack.-7z/%5BTHUG2%5D%20HQ%20Xbox%20Video%20and%20Audio%20Pack.7z"

# ---------- Helpers ----------

log()  { printf '\033[1;34m[install]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m[error]\033[0m %s\n' "$*" >&2; exit 1; }
ask_sudo() {
  log "About to run as root: $*"
  sudo "$@"
}

usage() {
  sed -n '2,/^$/p' "$0" | sed 's/^# \?//'
  exit 1
}

# Detect primary display resolution (used for Clownjob'd's 16:9 pillarbox math)
detect_resolution() {
  local res
  res=$(xrandr 2>/dev/null | awk '/\*/ {print $1; exit}')
  if [[ -n "$res" ]]; then
    THUG2_RES_W="${THUG2_RES_W:-${res%x*}}"
    THUG2_RES_H="${THUG2_RES_H:-${res#*x}}"
  else
    THUG2_RES_W="${THUG2_RES_W:-1920}"
    THUG2_RES_H="${THUG2_RES_H:-1080}"
  fi
  log "Detected display resolution: ${THUG2_RES_W}x${THUG2_RES_H}"
}

# ---------- Argument parsing ----------

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --iso)            ISO_PATH="$2";        shift 2 ;;
      --clownjobd-zip)  CLOWNJOBD_ZIP="$2";   shift 2 ;;
      --xbox-hq)        XBOX_HQ_PATH="$2";    shift 2 ;;
      --hq-textures)    HQ_TEXTURES_ZIP="$2"; shift 2 ;;
      --install-dir)    INSTALL_DIR="$2";     shift 2 ;;
      -h|--help)        usage ;;
      *) err "Unknown argument: $1" ;;
    esac
  done

  [[ -z "$ISO_PATH" || ! -f "$ISO_PATH" ]] && err "Need --iso pointing to an existing THUG2 PC ISO file"
  [[ -z "$CLOWNJOBD_ZIP" || ! -f "$CLOWNJOBD_ZIP" ]] && err "Need --clownjobd-zip pointing to THUG2_ClownJob'd_v1.3.zip (or similar)"
}

# ---------- Prerequisite checks ----------

check_prereqs() {
  log "Checking prerequisites..."
  [[ "$(uname -s)" == "Linux" ]] || err "Linux only"
  command -v curl  >/dev/null || err "curl required"
  command -v unzip >/dev/null || err "unzip required"
  command -v tar   >/dev/null || err "tar required"
}

# ---------- System packages (Fedora) ----------

install_packages() {
  log "Installing system packages (will prompt for sudo)..."
  local pkgs=(
    wine.i686 wine-core.i686 wine-pulseaudio.i686 wine-alsa.i686
    winetricks
    p7zip p7zip-plugins
    msitools
    cabextract
    python3-evdev          # for the controller trigger/bumper bridge
  )
  ask_sudo dnf install -y "${pkgs[@]}"
}

# ---------- Fedora d3d9 symlink workaround ----------
# wine-core.i686 ships wine-d3d9.dll but the alternatives system fails to
# create the d3d9.dll symlink. Without this, 32-bit games can't load D3D9.

fix_d3d9_symlinks() {
  log "Patching Fedora wine d3d9 symlinks..."
  local wow64_i386=/usr/lib/wine-wow64/wine/i386-windows
  local legacy_i386=/usr/lib/wine/i386-windows

  if [[ -f "$wow64_i386/wine-d3d9.dll" && ! -e "$wow64_i386/d3d9.dll" ]]; then
    ask_sudo ln -sf wine-d3d9.dll "$wow64_i386/d3d9.dll"
  fi
  if [[ ! -d "$legacy_i386" ]]; then
    ask_sudo mkdir -p "$legacy_i386"
  fi
  if [[ -f "$wow64_i386/wine-d3d9.dll" && ! -e "$legacy_i386/d3d9.dll" ]]; then
    ask_sudo ln -sf "$wow64_i386/wine-d3d9.dll" "$legacy_i386/d3d9.dll"
  fi
}

# ---------- Wine prefix ----------

init_wine_prefix() {
  if [[ -d "$WINE_PREFIX" ]]; then
    log "Wine prefix $WINE_PREFIX already exists; skipping init"
    return
  fi
  log "Creating 32-bit Wine prefix at $WINE_PREFIX..."
  WINEARCH=win32 WINEPREFIX="$WINE_PREFIX" wine32 wineboot
}

install_dxvk() {
  log "Installing DXVK $DXVK_VERSION into prefix..."
  mkdir -p "$WORK_DIR"
  local archive="$WORK_DIR/dxvk-${DXVK_VERSION}.tar.gz"
  if [[ ! -f "$archive" ]]; then
    curl -sL -o "$archive" \
      "https://github.com/doitsujin/dxvk/releases/download/v${DXVK_VERSION}/dxvk-${DXVK_VERSION}.tar.gz"
  fi
  tar xzf "$archive" -C "$WORK_DIR"
  cp "$WORK_DIR/dxvk-${DXVK_VERSION}/x32/"*.dll "$WINE_PREFIX/drive_c/windows/system32/"

  log "Setting DLL overrides to native for DXVK..."
  for dll in d3d9 d3d10core d3d11 dxgi; do
    WINEPREFIX="$WINE_PREFIX" wine32 reg add \
      "HKCU\\Software\\Wine\\DllOverrides" /v "$dll" /d native /f >/dev/null 2>&1
  done
}

install_winetricks_components() {
  log "Installing d3dx9, native dinput8, PulseAudio backend..."
  WINEPREFIX="$WINE_PREFIX" WINE=wine32 WINESERVER=wineserver32 \
    winetricks -q d3dx9 dinput8 sound=pulse
}

# ---------- Game data extraction (shared between vanilla + clownjobd) ----------

extract_shared_game_data() {
  local shared="$INSTALL_DIR/shared"
  if [[ -d "$shared/Data" && -f "$shared/Data/streams/pcm/pcm.dat" ]]; then
    log "Shared Data already extracted at $shared/Data; skipping"
    cp "$WORK_DIR/iso/Setup/Data/Game/THUG2.exe"   "$WORK_DIR/" 2>/dev/null || true
    cp "$WORK_DIR/iso/Setup/Data/Game/binkw32.dll" "$WORK_DIR/" 2>/dev/null || true
    cp "$WORK_DIR/iso/Setup/Data/Launcher.exe"     "$WORK_DIR/" 2>/dev/null || true
    return
  fi

  log "Extracting shared game data..."
  mkdir -p "$shared" "$WORK_DIR/iso" "$WORK_DIR/msi"

  log "  -> extracting ISO with 7z (~2GB, takes a minute)..."
  7z x -y "$ISO_PATH" -o"$WORK_DIR/iso" >/dev/null

  # Stash the vanilla EXE + binkw32 + config Launcher for the vanilla profile to use later.
  # Launcher.exe is the config tool that binds the controller (writes gp0_* keys to the
  # registry under HKCU\Software\Activision\Tony Hawk's Underground 2\Settings).
  cp "$WORK_DIR/iso/Setup/Data/Game/THUG2.exe"   "$WORK_DIR/"
  cp "$WORK_DIR/iso/Setup/Data/Game/binkw32.dll" "$WORK_DIR/"
  cp "$WORK_DIR/iso/Setup/Data/Launcher.exe"     "$WORK_DIR/" 2>/dev/null \
    || warn "Launcher.exe not found in ISO at Setup/Data/ — controller config tool unavailable for this rip"

  log "  -> extracting MSI for complete Data tree..."
  ( cd "$WORK_DIR/msi" && msiextract "$WORK_DIR/iso/Tony Hawk's Underground 2.msi" >/dev/null )

  # msiextract uses colons in directory names from the MSI table
  local src="$WORK_DIR/msi/.:Setup/Tony Hawk's Underground 2:Data/Game/Data"
  [[ -d "$src" ]] || err "MSI extraction did not produce expected Data tree at: $src"

  mkdir -p "$shared/Data"
  cp -rn "$src/." "$shared/Data/"
  log "  -> shared Data ready"
}

# ---------- Xbox HQ Audio/Video Pack (drop-in upgrade of shared/Data) ----------
# Replaces music, PCM audio, and cutscene videos with the higher-quality Xbox versions.
# Both profiles benefit since they share Data/ via symlink.
# ~866 MB download from archive.org if --xbox-hq not provided.

install_xbox_hq_pack() {
  local shared="$INSTALL_DIR/shared"
  local archive

  if [[ -n "$XBOX_HQ_PATH" ]]; then
    archive="$XBOX_HQ_PATH"
    [[ -f "$archive" ]] || err "--xbox-hq path does not exist: $archive"
  else
    archive="$WORK_DIR/THUG2_HQ_Xbox.7z"
    if [[ ! -f "$archive" ]]; then
      log "Downloading Xbox HQ Audio/Video Pack from archive.org (~866 MB)..."
      curl -L -o "$archive" "$XBOX_HQ_URL"
    fi
  fi

  # Idempotency check: look for a Xbox-version bik that wouldn't exist in the base install.
  # Can't use pcm.wad size anymore since we deliberately keep the PC version.
  local marker="$shared/Data/streams/music/8541624c.bik"
  if [[ -f "$marker" ]] && [[ $(stat -c%s "$marker") -gt 8000000 ]]; then
    log "Xbox HQ pack already applied (Xbox-size bik present); skipping"
    return
  fi

  log "Extracting + applying Xbox HQ pack..."
  rm -rf "$WORK_DIR/xbox_hq"
  mkdir -p "$WORK_DIR/xbox_hq"
  7z x -y -o"$WORK_DIR/xbox_hq" "$archive" >/dev/null
  [[ -d "$WORK_DIR/xbox_hq/Game/Data" ]] || err "Unexpected layout in Xbox HQ pack: no Game/Data/"

  # KNOWN GOTCHA: Xbox pcm.wad/pcm.dat break in-engine character dialog
  # (Tony, Rodney Mullen tutorial voice, etc.). The IDs in Xbox PCM data
  # don't map to PC scripts. Other Xbox mods (THUGXboxMod) document this.
  # Remove them from the staged copy so the PC versions stay in place.
  rm -f "$WORK_DIR/xbox_hq/Game/Data/streams/pcm/pcm.wad"
  rm -f "$WORK_DIR/xbox_hq/Game/Data/streams/pcm/pcm.dat"

  cp -rf "$WORK_DIR/xbox_hq/Game/Data/." "$shared/Data/"
  log "  -> Xbox HQ pack applied (kept original PC pcm.wad/pcm.dat to preserve dialog)"
}

# ---------- HQ Classic Level Textures (Zure v2.1) — optional ----------
# Improves School, Downhill Jam, Philly, Canada, Los Angeles textures + scene archives.
# User must source from PHUN/ModDB/ThMods; we don't auto-download (not a stable URL).

install_hq_textures() {
  [[ -z "$HQ_TEXTURES_ZIP" ]] && { log "Skipping HQ textures (no --hq-textures provided)"; return; }
  [[ ! -f "$HQ_TEXTURES_ZIP" ]] && err "--hq-textures path does not exist: $HQ_TEXTURES_ZIP"

  local shared="$INSTALL_DIR/shared"
  log "Applying HQ Classic Level Textures from $HQ_TEXTURES_ZIP..."

  rm -rf "$WORK_DIR/hqtex"
  mkdir -p "$WORK_DIR/hqtex"
  unzip -q -o "$HQ_TEXTURES_ZIP" -d "$WORK_DIR/hqtex"

  # Linux is case-sensitive; zip has "Levels/" but our tree has "levels/".
  [[ -d "$WORK_DIR/hqtex/Levels" ]] && mv "$WORK_DIR/hqtex/Levels" "$WORK_DIR/hqtex/levels"

  # The zip also creates "SC_sky" (lowercase y) but our existing dir is "SC_Sky".
  # Move the texture into the existing dir to avoid a duplicate ghost directory.
  if [[ -d "$WORK_DIR/hqtex/levels/SC_sky" ]]; then
    mkdir -p "$shared/Data/levels/SC_Sky"
    mv "$WORK_DIR/hqtex/levels/SC_sky/"* "$shared/Data/levels/SC_Sky/" 2>/dev/null || true
    rmdir "$WORK_DIR/hqtex/levels/SC_sky" 2>/dev/null || true
  fi

  cp -rf "$WORK_DIR/hqtex/levels/." "$shared/Data/levels/"
  cp -rf "$WORK_DIR/hqtex/pre/."    "$shared/Data/pre/"
  log "  -> HQ textures applied to CA, DJ, LA, PH, SC, SC_Sky"
}

# ---------- Vanilla profile (THUG2.exe + WidescreenFixesPack) ----------

build_vanilla_profile() {
  local target="$INSTALL_DIR/vanilla"
  log "Building vanilla profile at $target..."
  mkdir -p "$target/scripts" "$target/Save"

  cp "$WORK_DIR/THUG2.exe"   "$target/"
  cp "$WORK_DIR/binkw32.dll" "$target/"
  # Config launcher (binds keyboard/gamepad) — used by configure-controls.sh
  cp "$WORK_DIR/Launcher.exe" "$target/" 2>/dev/null || warn "  -> no Launcher.exe staged; controller config tool unavailable"

  # Symlink Data to shared
  rm -f "$target/Data"
  ln -s "../shared/Data" "$target/Data"

  # Install WidescreenFixesPack (drop-in ASI loader + ASI plugin).
  # CRITICAL: WSFix ships its Ultimate ASI Loader as dinput8.dll, but a dinput8 proxy in
  # the game folder BREAKS HID-joystick enumeration, killing native controller support.
  # THUG2.exe also imports winmm.dll, so we install the SAME loader as winmm.dll instead.
  # That keeps dinput8 Wine-native (controller works) while WSFix still loads. Confirmed
  # working: ultrawide FOV/HUD + native analog Xbox 360 pad simultaneously.
  local wsfix_zip="$WORK_DIR/TonyHawksUnderground2.WidescreenFix.zip"
  if [[ ! -f "$wsfix_zip" ]]; then
    log "  -> downloading WidescreenFixesPack..."
    curl -sL -o "$wsfix_zip" \
      "https://github.com/ThirteenAG/WidescreenFixesPack/releases/download/${WSFIX_TAG}/TonyHawksUnderground2.WidescreenFix.zip"
  fi
  rm -rf "$WORK_DIR/wsfix"
  unzip -q -o "$wsfix_zip" -d "$WORK_DIR/wsfix"
  cp "$WORK_DIR/wsfix/Game/dinput8.dll" "$target/winmm.dll"   # <-- renamed proxy, NOT dinput8
  cp "$WORK_DIR/wsfix/Game/scripts/"* "$target/scripts/"

  # Make Wine load our winmm proxy (it forwards to the builtin winmm).
  log "  -> setting winmm override (native,builtin) for the WSFix proxy"
  WINEPREFIX="$WINE_PREFIX" wine32 reg add \
    "HKCU\\Software\\Wine\\DllOverrides" /v winmm /d "native,builtin" /f >/dev/null 2>&1
}

# ---------- Clownjob'd profile (THUGTWO.exe + ClownJob'd + WidescreenFixesPack) ----------
# Clownjob'd alone tops out at 21:9 and looks bad on ultrawide. Combining with WSFix:
# Clownjob'd renders at native res + tells the game it's 16:9; WSFix corrects FOV/HUD
# from 16:9 to actual monitor aspect. Tested and works perfectly on 32:9 (5120x1440).

build_clownjobd_profile() {
  local target="$INSTALL_DIR/clownjobd"
  log "Building Clownjob'd profile at $target..."
  mkdir -p "$target/Save" "$target/scripts"

  cp "$WORK_DIR/binkw32.dll" "$target/"

  # Symlink Data to shared
  rm -f "$target/Data"
  ln -s "../shared/Data" "$target/Data"

  # Extract Clownjob'd zip
  rm -rf "$WORK_DIR/clownjobd"
  unzip -q -o "$CLOWNJOBD_ZIP" -d "$WORK_DIR/clownjobd"

  # The zip's structure varies; find THUGTWO.exe + DLL + INI inside
  local thugtwo dll ini
  thugtwo=$(find "$WORK_DIR/clownjobd" -iname "THUGTWO.exe" | head -1)
  dll=$(find "$WORK_DIR/clownjobd" -iname "ClownJob*.dll" | head -1)
  ini=$(find "$WORK_DIR/clownjobd" -iname "ClownJob*.ini" | head -1)
  [[ -z "$thugtwo" || -z "$dll" || -z "$ini" ]] && err "Couldn't find THUGTWO.exe / ClownJob'd.dll / ClownJob'd.ini in $CLOWNJOBD_ZIP"

  cp "$thugtwo" "$target/"
  cp "$dll"     "$target/ClownJob'd.dll"
  cp "$ini"     "$target/ClownJob'd.ini"

  # Layer WSFix on top so widescreen works correctly past 21:9.
  # Clownjob'd config: render at native res, claim 16:9 (well-defined for WSFix).
  # WSFix config: default (FixHUD=1, FixFOV=1) — corrects from 16:9 to actual aspect.
  local wsfix_zip="$WORK_DIR/TonyHawksUnderground2.WidescreenFix.zip"
  if [[ ! -f "$wsfix_zip" ]]; then
    curl -sL -o "$wsfix_zip" \
      "https://github.com/ThirteenAG/WidescreenFixesPack/releases/download/${WSFIX_TAG}/TonyHawksUnderground2.WidescreenFix.zip"
  fi
  rm -rf "$WORK_DIR/wsfix"
  unzip -q -o "$wsfix_zip" -d "$WORK_DIR/wsfix"
  cp "$WORK_DIR/wsfix/Game/dinput8.dll" "$target/"
  cp "$WORK_DIR/wsfix/Game/scripts/"* "$target/scripts/"

  # Clownjob'd: native res, ScreenMode=2 (16:9, what WSFix's FOV math expects)
  log "  -> Clownjob'd: ${THUG2_RES_W}x${THUG2_RES_H} ScreenMode=2 (16:9) + WSFix layer"
  sed -i \
    -e "s/^Width=.*/Width=${THUG2_RES_W}/" \
    -e "s/^Height=.*/Height=${THUG2_RES_H}/" \
    -e "s/^ScreenMode=.*/ScreenMode=2/" \
    -e "s/^BorderlessWindow=.*/BorderlessWindow=1/" \
    -e "s/^Windowed=.*/Windowed=0/" \
    -e "s/^WindowPositionMode=.*/WindowPositionMode=1/" \
    -e "s/^WindowPositionX=.*/WindowPositionX=0/" \
    -e "s/^WindowPositionY=.*/WindowPositionY=0/" \
    "$target/ClownJob'd.ini"
}

# ---------- Controller (native analog, PS2-faithful) ----------
# THUG2 reads gamepad bindings from the registry; we import a known-good PS2-style
# Xbox-360 map (gp0_*/k0_*) plus the Wine "Override" that splits the 360 pad's triggers
# into independent axes. A small user-space bridge (thug2-trigger-bridge.py) reads the
# pad's triggers + bumpers from the Linux kernel and feeds the game keys, because THUG2's
# native DInput can't use split triggers (reads them as always-held) and has no 2-button
# combo for walk. PS2 layout: A/B/X/Y = Ollie/Grab/Flip/Grind; LT/RT = Nollie/Switch
# (both = Level Out); LB/RB = spin (both = get off board); L3 = Focus; R3 = Camera; sticks native.
#
# NOTE: the gp0_* values + Override were captured under Lutris Wine-GE 8.0 (where the whole
# controller setup is validated). On a different Wine build the Override button layout can
# differ; if the pad misbehaves, run configure-controls.sh and re-bind in the launcher.
setup_controller() {
  local reg_override="$SCRIPT_DIR/tools/controls/thug2-joystick-override.reg"
  local reg_settings="$SCRIPT_DIR/tools/controls/thug2-settings.reg"
  local bridge_src="$SCRIPT_DIR/tools/trigger-bridge/thug2-trigger-bridge.py"

  log "Setting up native PS2-style controller..."
  if [[ -f "$reg_override" && -f "$reg_settings" ]]; then
    log "  -> importing controller bindings + trigger-split Override"
    WINEPREFIX="$WINE_PREFIX" wine32 reg import "$reg_override" >/dev/null 2>&1 || warn "    override import failed"
    WINEPREFIX="$WINE_PREFIX" wine32 reg import "$reg_settings" >/dev/null 2>&1 || warn "    settings import failed"
  else
    warn "  -> controller .reg files missing in tools/controls — skip (use configure-controls.sh to bind)"
  fi

  if [[ -f "$bridge_src" ]]; then
    cp "$bridge_src" "$INSTALL_DIR/thug2-trigger-bridge.py"
    chmod +x "$INSTALL_DIR/thug2-trigger-bridge.py"
    log "  -> installed trigger-bridge (needs python3 + python-evdev; user ACL on /dev/input + /dev/uinput)"
  else
    warn "  -> trigger-bridge script missing — LT/RT/LB/RB remap won't run"
  fi
}

# ---------- Launcher scripts ----------

write_launchers() {
  cat > "$INSTALL_DIR/launch-vanilla.sh" <<EOF
#!/usr/bin/env bash
# Vanilla THUG2 + native analog controller + PS2 trigger/bumper bridge.
# The bridge starts with the game and stops when it exits.
BRIDGE="$INSTALL_DIR/thug2-trigger-bridge.py"
bridge_pid=""
if [ -e "\$BRIDGE" ] && command -v python3 >/dev/null 2>&1; then
  python3 "\$BRIDGE" & bridge_pid=\$!
  trap '[ -n "\$bridge_pid" ] && kill "\$bridge_pid" 2>/dev/null || true' EXIT
fi
cd "$INSTALL_DIR/vanilla" && env WINEPREFIX="$WINE_PREFIX" wine32 THUG2.exe "\$@"
EOF
  chmod +x "$INSTALL_DIR/launch-vanilla.sh"

  cat > "$INSTALL_DIR/launch-clownjobd.sh" <<EOF
#!/usr/bin/env bash
cd "$INSTALL_DIR/clownjobd" && exec env WINEPREFIX="$WINE_PREFIX" wine32 THUGTWO.exe "\$@"
EOF
  chmod +x "$INSTALL_DIR/launch-clownjobd.sh"

  # Controller config tool. Run once to bind a gamepad (or rebind keyboard).
  # Bindings persist in the prefix registry and are read by the vanilla profile.
  cat > "$INSTALL_DIR/configure-controls.sh" <<EOF
#!/usr/bin/env bash
# THUG2 config launcher — bind your keyboard/gamepad here, then Save.
# Plug the controller in BEFORE launching (don't hotplug; THUG2 can crash on it).
cd "$INSTALL_DIR/vanilla" && exec env WINEPREFIX="$WINE_PREFIX" wine32 Launcher.exe "\$@"
EOF
  chmod +x "$INSTALL_DIR/configure-controls.sh"

  log "Launchers:"
  log "  $INSTALL_DIR/launch-vanilla.sh     (best display on ultrawide; native analog controller)"
  log "  $INSTALL_DIR/launch-clownjobd.sh   (Clownjob'd features, pillarboxed; keyboard only)"
  log "  $INSTALL_DIR/configure-controls.sh (run ONCE to bind your gamepad)"
}

# ---------- Orchestration ----------

main() {
  parse_args "$@"
  check_prereqs
  detect_resolution
  install_packages
  fix_d3d9_symlinks
  init_wine_prefix
  install_dxvk
  install_winetricks_components

  extract_shared_game_data
  install_xbox_hq_pack
  install_hq_textures
  build_vanilla_profile
  build_clownjobd_profile
  setup_controller
  write_launchers

  log "Done."
  log "To use a controller (native analog support): plug it in, then run ONCE:"
  log "  $INSTALL_DIR/configure-controls.sh   (bind each button in the launcher, Save)"
  log "Then launch with one of:"
  log "  $INSTALL_DIR/launch-vanilla.sh     (ultrawide + controller — recommended)"
  log "  $INSTALL_DIR/launch-clownjobd.sh   (Clownjob'd features, keyboard)"
}

main "$@"
