#!/usr/bin/env bash
#
# revert-acquire-hq.sh — fetch the optional HQ packs (Xbox HQ audio + HQ level
# textures) into the paths `revert build` already reads. These are community /
# derivative packs that Revert does NOT host: set HQ_*_URL in revert.conf to
# auto-download, or leave a URL empty and drop the file at the printed path
# (bring-your-own). After acquiring, `revert build` applies them automatically.
#
#   revert acquire-hq [audio|textures|all]     (default: all)
#
set -euo pipefail

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

log()  { printf '\033[1;34m[acquire-hq]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[acquire-hq:warn]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m[acquire-hq:error]\033[0m %s\n' "$*" >&2; exit 1; }

verify_sha() {  # <file> <expected-sha256|"">
  local f="$1" want="${2:-}"
  [[ -z "$want" ]] && return 0
  command -v sha256sum >/dev/null || { warn "sha256sum not found — skipping integrity check"; return 0; }
  local got; got="$(sha256sum "$f" | awk '{print $1}')"
  [[ "$got" == "$want" ]] || err "checksum mismatch for $(basename "$f"): got $got, want $want"
  log "checksum OK ($(basename "$f"))"
}

download() {  # <url> <dest>
  command -v curl >/dev/null || err "curl required to download (or bring your own file)"
  log "downloading $(basename "$2") ..."
  curl -fL --retry 3 -o "$2" "$1" || err "download failed: $1"
}

acquire_audio() {
  if [[ -f "$HQ_AUDIO_PACK" ]]; then log "HQ audio already present: $HQ_AUDIO_PACK"; return 0; fi
  if [[ -n "${HQ_AUDIO_URL:-}" ]]; then
    mkdir -p "$(dirname "$HQ_AUDIO_PACK")"
    download "$HQ_AUDIO_URL" "$HQ_AUDIO_PACK"
    verify_sha "$HQ_AUDIO_PACK" "${HQ_AUDIO_SHA256:-}"
    log "HQ audio ready -> run: revert build"
  else
    warn "no HQ_AUDIO_URL set — Revert does not host this pack. Bring your own:"
    echo "   1) obtain the THUG2 Xbox HQ audio pack (.7z)"
    echo "   2) place it at:  $HQ_AUDIO_PACK"
    echo "   3) run:          revert build   (it applies automatically)"
    echo "   ...or set HQ_AUDIO_URL in revert.conf and re-run: revert acquire-hq audio"
  fi
}

acquire_textures() {
  mkdir -p "$HQ_TEXTURES_BLOB"
  if compgen -G "$HQ_TEXTURES_BLOB/*.tex.xbx" >/dev/null; then
    log "HQ textures already present: $HQ_TEXTURES_BLOB"; return 0
  fi
  if [[ -n "${HQ_TEXTURES_URL:-}" ]]; then
    local tmp; tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' RETURN
    local arc="$tmp/hqtex.bin"
    download "$HQ_TEXTURES_URL" "$arc"
    verify_sha "$arc" "${HQ_TEXTURES_SHA256:-}"
    log "staging *.tex.xbx into $HQ_TEXTURES_BLOB"
    if command -v 7z >/dev/null && 7z l "$arc" >/dev/null 2>&1; then
      7z e -y -o"$HQ_TEXTURES_BLOB" "$arc" '*.tex.xbx' >/dev/null
    elif command -v unzip >/dev/null && unzip -l "$arc" >/dev/null 2>&1; then
      unzip -jo "$arc" '*.tex.xbx' -d "$HQ_TEXTURES_BLOB" >/dev/null
    else
      warn "unknown archive format — copying as-is; ensure blob/*.tex.xbx names match inject.list"
      cp "$arc" "$HQ_TEXTURES_BLOB/"
    fi
    log "HQ textures ready -> run: revert build"
  else
    warn "no HQ_TEXTURES_URL set — Revert does not host this pack. Bring your own:"
    echo "   1) obtain the HQ level-textures pack (CA/DJ/SC .tex.xbx)"
    echo "   2) place the files in:  $HQ_TEXTURES_BLOB/"
    echo "      (named ca.tex.xbx, DJ.tex.xbx, SC.tex.xbx — see the mod's inject.list)"
    echo "   3) run:                 revert build   (hq-level-textures applies automatically)"
    echo "   ...or set HQ_TEXTURES_URL in revert.conf and re-run: revert acquire-hq textures"
  fi
}

case "${1:-all}" in
  audio)    acquire_audio;;
  textures) acquire_textures;;
  all)      acquire_audio; acquire_textures;;
  *) err "usage: revert acquire-hq [audio|textures|all]";;
esac
