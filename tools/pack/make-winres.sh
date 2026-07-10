#!/usr/bin/env bash
#
# make-winres.sh — generate the Windows PE resources (app icon + version info) that get
# linked into revert.exe and revert-gui.exe.
#
# Go links any `*.syso` in a main package's directory. The `_windows` filename suffix is a
# build constraint, so these resources are picked up for GOOS=windows and ignored entirely
# on Linux and the Steam Deck.
#
#   tools/pack/make-winres.sh [VERSION]
#
# VERSION defaults to $VERSION, else "dev". Rerun it whenever the icon changes; the packer
# calls it automatically so a release bundle always carries the right version.
#
# The generated .syso files are committed, so a plain `GOOS=windows go build` (and anyone
# without goversioninfo installed) still produces an iconned binary.
set -euo pipefail

ROOT="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)"
cd "$ROOT"

VERSION="${1:-${VERSION:-dev}}"
ICON="$ROOT/tools/pack/icon/revert.ico"
BASE="$ROOT/tools/pack/icon/versioninfo.json"

log() { printf '\033[1;34m[winres]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[winres:error]\033[0m %s\n' "$*" >&2; exit 1; }

[ -f "$ICON" ] || die "missing $ICON (regenerate: python3 tools/pack/icon/make_app_icon.py tools/pack/icon)"

GV="$(command -v goversioninfo || true)"
[ -z "$GV" ] && [ -x "$(go env GOPATH)/bin/goversioninfo" ] && GV="$(go env GOPATH)/bin/goversioninfo"
[ -n "$GV" ] || die "goversioninfo not found. Install it:
  go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest"

# Split "v1.4.0" into the numeric fields the Windows version resource requires. A "dev"
# build (or any non-numeric tag) reports 0.0.0.0, which is what an unversioned binary
# should say rather than a flattering lie.
num="${VERSION#v}"
if [[ "$num" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
  MAJ="${BASH_REMATCH[1]}"; MIN="${BASH_REMATCH[2]}"; PAT="${BASH_REMATCH[3]}"
  # Windows wants the *string* fields as x.y.z.b too; a "v"-prefixed git tag makes
  # goversioninfo warn and reads oddly in Explorer's Properties dialog.
  STRVER="${MAJ}.${MIN}.${PAT}.0"
else
  MAJ=0; MIN=0; PAT=0
  STRVER="0.0.0.0"
fi

log "version ${VERSION}  ->  ${STRVER}"

# name | main package dir | FileDescription (what Explorer's Properties dialog shows)
gen() {
  local exe="$1" dir="$2" desc="$3"
  log "  $exe"
  ( cd "$dir" && "$GV" \
      -icon="$ICON" \
      -o "resource_windows.syso" \
      -company "Violet Vandal" \
      -product-name "THUG2: Violet Vandal Edition" \
      -description "$desc" \
      -internal-name "${exe%.exe}" \
      -original-name "$exe" \
      -copyright "Copyright (c) Violet Vandal. MIT licensed." \
      -comment "${VERSION} — ships tooling, never game data. https://github.com/violetvandal/revert" \
      -file-version "$STRVER" \
      -product-version "$STRVER" \
      -ver-major "$MAJ" -ver-minor "$MIN" -ver-patch "$PAT" -ver-build 0 \
      -product-ver-major "$MAJ" -product-ver-minor "$MIN" -product-ver-patch "$PAT" -product-ver-build 0 \
      "$BASE" )
}

gen "revert.exe"       "$ROOT/cmd/revert"       "Revert (command line)"
gen "revert-gui.exe"   "$ROOT/gui"              "Revert"
gen "vv-padbridge.exe" "$ROOT/cmd/vv-padbridge" "Revert controller bridge"

# thugkit lives in its own repo (a submodule of the public one). Its .syso is committed
# there, so a standalone `go build` of thugkit is iconned too; skip quietly if the
# submodule isn't checked out.
if [ -d "$ROOT/tools/thugkit/cmd/thugkit" ]; then
  gen "thugkit.exe" "$ROOT/tools/thugkit/cmd/thugkit" "Revert build core"
fi

log "done. Generated:"
git -C "$ROOT" status --short -- '*resource_windows.syso' 2>/dev/null | sed 's/^/  /' || true
find "$ROOT" -name 'resource_windows.syso' -not -path '*/game-*' -printf '  %p (%s bytes)\n'
