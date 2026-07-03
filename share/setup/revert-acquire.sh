#!/usr/bin/env bash
#
# revert-acquire.sh — turn YOUR OWN THUG2 copy into the clean pristine base that
# `revert build` derives the edition from. Revert ships tooling, never game data:
# you must own THUG2 (disc/ISO/an installed folder).
#
#   revert acquire-game-data --folder <dir>          # an installed/extracted THUG2 (Steam/GOG/disc install)
#   revert acquire-game-data --iso <cd1> [--iso <cd2> --iso <cd3>]   # from disc ISOs
#   revert acquire-game-data --disc-dir <dir>        # a dir containing the CD ISOs
#   [--force]                                        # overwrite an existing pristine
#
# Output: $PRISTINE_DIR (from revert.conf) with Data/ + root exes, verified to
# contain Data/pre. The folder path is the most reliable; the ISO path follows the
# documented 3-disc msiextract recipe (see game-pristine-us/PRISTINE_README.txt).
#
set -euo pipefail

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

log()  { printf '\033[1;34m[acquire]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[acquire:warn]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m[acquire:error]\033[0m %s\n' "$*" >&2; exit 1; }

FOLDER=""; ISOS=(); DISC_DIR=""; MSI=""; FORCE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --folder)   FOLDER="$2"; shift 2;;
    --iso)      ISOS+=("$2"); shift 2;;
    --disc-dir) DISC_DIR="$2"; shift 2;;
    --msi)      MSI="$2"; shift 2;;
    --force)    FORCE=1; shift;;
    *) err "unknown arg '$1'";;
  esac
done

dest="$PRISTINE_DIR"
if [[ -d "$dest/Data/pre" && $FORCE -eq 0 ]]; then
  err "pristine already exists: $dest  (use --force to rebuild it)"
fi
mkdir -p "$dest"

verify() {
  local n; n=$(ls "$dest"/Data/pre/*.prx 2>/dev/null | wc -l)
  [[ -d "$dest/Data/pre" ]] || err "result has no Data/pre — acquisition failed"
  [[ -f "$dest/THUG2.exe" ]] || warn "no THUG2.exe at root (you'll need to supply a no-CD exe for builds)"
  log "pristine ready: $dest  ($n .prx in Data/pre)"
  [[ "$n" -ge 100 ]] || warn "expected ~115 .prx in Data/pre, found $n — copy may be incomplete"
  if [[ -f "$dest/SHA256SUMS.txt" ]]; then
    log "verifying SHA256SUMS.txt ..."
    ( cd "$dest" && sha256sum -c SHA256SUMS.txt >/dev/null 2>&1 ) && log "  checksums OK" || warn "  checksum mismatch (a non-pristine source is fine for building, just not bit-identical)"
  fi
}

# ---- folder source (installed / extracted THUG2) ------------------------------
from_folder() {
  local src="$1"
  [[ -d "$src/Data/pre" ]] || err "not a THUG2 install (no Data/pre): $src"
  log "copying from folder: $src"
  if command -v rsync >/dev/null; then
    rsync -a "$src/Data/" "$dest/Data/"
  else
    cp -a "$src/Data/." "$dest/Data/"
  fi
  # root loose files
  local f
  for f in THUG2.exe binkw32.dll gdiplus.dll THUG2.ico Launcher.exe Launcher_fr.exe Launcher_gr.exe; do
    [[ -f "$src/$f" ]] && cp -f "$src/$f" "$dest/"
  done
  cp -f "$src"/*.url "$dest/" 2>/dev/null || true
}

# ---- ISO source (3-disc msiextract recipe) ------------------------------------
from_isos() {
  command -v 7z >/dev/null    || err "7z required for ISO extraction (dnf install p7zip p7zip-plugins)"
  command -v msiextract >/dev/null || err "msiextract required (dnf install msitools)"
  local work; work="$(mktemp -d)"; trap 'rm -rf "$work"' RETURN
  local i=0
  for iso in "${ISOS[@]}"; do
    [[ -f "$iso" ]] || err "ISO not found: $iso"
    i=$((i+1)); log "extracting CD$i: $(basename "$iso")"
    7z x -y -o"$work/cd$i" "$iso" >/dev/null || err "7z failed on $iso"
  done
  # find the MSI + stage every CAB from all discs alongside it
  local msi; msi="${MSI:-$(find "$work" -iname '*.msi' | head -1)}"
  [[ -n "$msi" && -f "$msi" ]] || err "no .msi found in the ISOs (point --msi at it)"
  local stage="$work/stage"; mkdir -p "$stage"
  cp -f "$msi" "$stage/"
  find "$work" -iname '*.cab' -exec cp -f {} "$stage/" \;
  log "msiextract ($(ls "$stage"/*.cab 2>/dev/null | wc -l) CABs staged)"
  ( cd "$stage" && msiextract "$(basename "$msi")" >/dev/null ) || err "msiextract failed"
  # the MSI lays down Game/Data/*
  local gamedata; gamedata="$(find "$stage" -type d -iname Data -path '*Game*' | head -1)"
  [[ -n "$gamedata" ]] || gamedata="$(find "$stage" -type d -iname pre -path '*Data*' | head -1 | xargs -r dirname)"
  [[ -n "$gamedata" && -d "$gamedata" ]] || err "extracted tree has no Game/Data — unexpected MSI layout"
  mkdir -p "$dest/Data"
  cp -a "$gamedata/." "$dest/Data/"
  # overlay the exes/movies the installer lays down outside the MSI (Setup/Data/...)
  local g
  for g in $(find "$work" -type d -ipath '*Setup/Data/Game' ); do
    cp -f "$g"/THUG2.exe "$g"/binkw32.dll "$dest/" 2>/dev/null || true
    [[ -d "$g/Data/movies/bik" ]] && { mkdir -p "$dest/Data/movies/bik"; cp -f "$g"/Data/movies/bik/*.bik "$dest/Data/movies/bik/" 2>/dev/null || true; }
  done
  for g in $(find "$work" -type d -ipath '*Setup/Data' ! -ipath '*Setup/Data/Game'); do
    cp -f "$g"/Launcher*.exe "$g"/gdiplus.dll "$g"/THUG2.ico "$dest/" 2>/dev/null || true
    cp -f "$g"/*.url "$dest/" 2>/dev/null || true
  done
}

if [[ -n "$FOLDER" ]]; then
  from_folder "$FOLDER"
elif [[ -n "$DISC_DIR" ]]; then
  mapfile -t ISOS < <(find "$DISC_DIR" -iname '*.iso' | sort)
  [[ ${#ISOS[@]} -gt 0 ]] || err "no ISOs in $DISC_DIR"
  from_isos
elif [[ ${#ISOS[@]} -gt 0 ]]; then
  from_isos
else
  err "specify a source: --folder <dir> | --iso <f> [...] | --disc-dir <dir>"
fi

verify
log "next: revert build   (then: revert run qol)"
