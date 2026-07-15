#!/usr/bin/env bash
#
# revert-acquire.sh — turn YOUR OWN THUG2 copy into the clean pristine base that
# `revert build` derives the edition from. Revert ships tooling, never game data:
# you must own THUG2 (disc/ISO/an installed folder).
#
#   revert acquire-game-data --folder <dir>          # an installed/extracted THUG2 (Steam/GOG/disc install)
#   revert acquire-game-data --iso <cd1> [--iso <cd2> --iso <cd3>]   # from disc ISOs
#   revert acquire-game-data --disc-dir <dir>        # a dir containing the CD ISOs
#   revert acquire-game-data --url <link>            # download a .zip/.7z/.iso/.tar.* you point us at
#   [--force]                                        # overwrite an existing pristine
#
# Output: $PRISTINE_DIR (from revert.conf) with Data/ + root exes, verified to
# contain Data/pre. The folder path is the most reliable; the ISO path follows the
# documented 3-disc msiextract recipe (see game-pristine-us/PRISTINE_README.txt).
#
# --url is a plain fetcher (like curl/wget): YOU supply the link, YOU are responsible
# for having the legal right to those files. Revert ships no game data and no sources —
# there is no default/bundled URL. The downloaded archive is routed into the same
# --folder / --iso pipeline once its contents are identified.
#
set -euo pipefail

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

log()  { printf '\033[1;34m[acquire]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[acquire:warn]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m[acquire:error]\033[0m %s\n' "$*" >&2; exit 1; }
note() { printf '\033[0;36m[acquire]\033[0m %s\n' "$*"; }
hsize(){ numfmt --to=iec "${1:-0}" 2>/dev/null || echo "${1:-0}"; }

FOLDER=""; ISOS=(); DISC_DIR=""; MSI=""; URL=""; FORCE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --folder)   FOLDER="$2"; shift 2;;
    --iso)      ISOS+=("$2"); shift 2;;
    --disc-dir) DISC_DIR="$2"; shift 2;;
    --msi)      MSI="$2"; shift 2;;
    --url)      URL="$2"; shift 2;;
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
  command -v 7z >/dev/null    || err "7z required for ISO extraction (dnf install p7zip p7zip-plugins / apt install p7zip-full)"
  command -v msiextract >/dev/null || err "msiextract required (dnf install msitools / apt install msitools)"
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

# ---- URL source (a plain fetcher — user-supplied link, no bundled sources) -----
# Work dir lives on the SAME filesystem as the pristine dest (a real disk, not a
# /tmp tmpfs that a 2–3 GB download would blow up). Override with REVERT_TMP=<dir>.
acquire_workdir() {
  local base="${REVERT_TMP:-$(dirname "$dest")}"
  mkdir -p "$base"
  mktemp -d "${base%/}/.revert-acquire.XXXXXX"
}

# fetch <url> <out> — resumable download with newline progress (streams over the GUI).
fetch() {
  local url="$1" out="$2" part="$2.part" total=0 rc=0
  command -v curl >/dev/null || command -v wget >/dev/null || err "need curl or wget to download from a URL"
  if command -v curl >/dev/null; then
    # Best-effort size for progress. Must not be fatal: a HEAD failure (e.g. 404)
    # would otherwise trip set -e/pipefail here, before the real error handling.
    total="$(curl -fsIL "$url" 2>/dev/null | tr -d '\r' | awk 'tolower($1)=="content-length:"{v=$2} END{print v+0}')" || total=0
    # -s silences curl's own carriage-return meter (garbles the GUI console); our
    # newline poller below reports progress instead. -S keeps real errors visible.
    curl -fsSL --retry 3 -C - -o "$part" "$url" &
  else
    wget -nv -c -O "$part" "$url" &
  fi
  local dlpid=$!
  ( local shown=""
    while kill -0 "$dlpid" 2>/dev/null; do
      sleep 3
      local cur; cur=$(stat -c%s "$part" 2>/dev/null || echo 0)
      if [[ "$total" -gt 0 ]]; then
        local pct=$(( cur * 100 / total ))
        [[ "$pct" != "$shown" ]] && { log "  downloading ${pct}%  ($(hsize "$cur") / $(hsize "$total"))"; shown="$pct"; }
      else
        log "  downloading $(hsize "$cur") ..."
      fi
    done ) &
  local ppid=$!
  wait "$dlpid" || rc=$?
  kill "$ppid" 2>/dev/null || true; wait "$ppid" 2>/dev/null || true
  [[ $rc -eq 0 ]] || err "download failed (exit $rc): $url"
  mv "$part" "$out"
  log "  downloaded $(hsize "$(stat -c%s "$out")"): $(basename "$out")"
}

from_url() {
  local url="$1"
  case "$url" in
    http://*|https://*|ftp://*) : ;;
    *) err "unsupported URL scheme (use http/https/ftp): $url";;
  esac
  note "You supplied this link — you are responsible for having the legal right to these files."
  note "Revert provides no game data and no sources; --url is just a downloader."
  local work; work="$(acquire_workdir)"; trap 'rm -rf "$work"' RETURN
  local name; name="$(basename "${url%%\?*}")"; [[ -n "$name" && "$name" != "/" ]] || name="download.bin"
  local dl="$work/$name"
  log "downloading game archive ..."
  fetch "$url" "$dl"

  # Route the download into the existing --folder / --iso pipeline by content.
  local x="$work/x"; mkdir -p "$x"; local lc="${name,,}"
  case "$lc" in
    *.iso)
      log "source is an ISO — extracting"; ISOS=("$dl"); from_isos; return;;
    *.tar|*.tar.gz|*.tgz|*.tar.xz|*.tar.bz2)
      log "extracting tarball ..."; tar -xf "$dl" -C "$x" || err "tar extract failed";;
    *.zip|*.7z|*.rar|*.001)
      command -v 7z >/dev/null || err "7z required to extract $name (dnf install p7zip p7zip-plugins / apt install p7zip-full)"
      log "extracting archive ..."; 7z x -y -o"$x" "$dl" >/dev/null || err "7z extract failed";;
    *)
      command -v 7z >/dev/null && 7z x -y -o"$x" "$dl" >/dev/null 2>&1 \
        || err "don't know how to extract '$name' (supported: .iso .zip .7z .rar .tar.*)";;
  esac

  # A THUG2 install (has Data/pre) or disc ISOs inside the extracted tree?
  local pre; pre="$(find "$x" -type d -ipath '*/Data/pre' 2>/dev/null | head -1)"
  if [[ -n "$pre" ]]; then
    log "found a THUG2 install inside the archive"
    from_folder "$(dirname "$(dirname "$pre")")"; return
  fi
  mapfile -t ISOS < <(find "$x" -iname '*.iso' 2>/dev/null | sort)
  if [[ ${#ISOS[@]} -gt 0 ]]; then
    log "found ${#ISOS[@]} ISO(s) inside the archive"; from_isos; return
  fi
  err "the downloaded archive has no THUG2 install (no Data/pre) and no ISOs inside"
}

if [[ -n "$URL" ]]; then
  from_url "$URL"
elif [[ -n "$FOLDER" ]]; then
  from_folder "$FOLDER"
elif [[ -n "$DISC_DIR" ]]; then
  mapfile -t ISOS < <(find "$DISC_DIR" -iname '*.iso' | sort)
  [[ ${#ISOS[@]} -gt 0 ]] || err "no ISOs in $DISC_DIR"
  from_isos
elif [[ ${#ISOS[@]} -gt 0 ]]; then
  from_isos
else
  err "specify a source: --folder <dir> | --iso <f> [...] | --disc-dir <dir> | --url <link>"
fi

verify
log "next: revert build   (then: revert run qol)"
