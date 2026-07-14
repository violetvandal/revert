#!/usr/bin/env bash
#
# build-windows-bundle.sh — maintainer command: cross-compile the whole Windows lane
# from Linux and zip a self-contained TOOLING bundle (never game data) that a Windows
# user extracts and runs. Produces dist/revert-windows-amd64.zip.
#
# Contents: revert.exe (front door) · revert-gui.exe · thugkit.exe (build core) ·
# ns.exe (skater-extractor helper) · vv-padbridge.exe (controller combos) · revert.conf ·
# the shipped .asi mods · the controller .reg · mods/ sources · docs · README-WINDOWS.txt.
#
# The user still supplies their own THUG2 copy, the no-CD exe, the WidescreenFix zip, and
# any HQ packs — exactly like the Linux/Deck lanes. No game data is ever bundled.
set -euo pipefail

ROOT="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)"
cd "$ROOT"

GOOS=windows GOARCH=amd64
export GOOS GOARCH CGO_ENABLED=0
# NOTE: deliberately NOT stripping (-s -w). A stripped Go binary looks like packed malware
# to Windows Defender's ML heuristic and gets false-positive flagged (Wacatac). Keeping the
# symbol table costs a few MB but markedly reduces the false positives. The real fix for a
# public release is Authenticode code-signing.
LDFLAGS=''

# The release tag this bundle represents. `revert update` compares it against the latest
# GitHub release to decide whether to self-update, so an unstamped ("dev") build refuses to
# update rather than risk downgrading itself. Override for a release build:
#
#   VERSION=v1.4.0 tools/pack/build-windows-bundle.sh
#
# Default: the tag at HEAD, else "dev" (the private dev root carries no tags).
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --exact-match 2>/dev/null || echo dev)}"
REVERT_LDFLAGS="-X github.com/violetvandal/revert/internal/core.Version=${VERSION}"

STAGE="$(mktemp -d)"
# The .syso resources are tracked, and stamping them for a release rewrites them in place.
# Restore the committed "dev" stamp on the way out, so a release build never leaves a dirty
# tree carrying a release version into the next commit.
STAMPED=0
cleanup() {
  rm -rf "$STAGE"
  [ "$STAMPED" = 1 ] && bash "$ROOT/tools/pack/make-winres.sh" dev >/dev/null 2>&1 || true
}
trap cleanup EXIT
OUT="$ROOT/dist"
ZIP="$OUT/revert-windows-amd64.zip"
mkdir -p "$OUT"

log() { printf '\033[1;34m[pack]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[pack:error]\033[0m %s\n' "$*" >&2; exit 1; }

# ── cross-compile the Go binaries ───────────────────────────────────────────────
log "version: ${VERSION}"
[ "$VERSION" = dev ] && log "(unstamped: 'revert update' will refuse to self-update. Set VERSION=vX.Y.Z for a release.)"

# App icon + version resource (Explorer's Properties dialog). Regenerated so the release
# bundle's PE version matches the tag. Best-effort: the .syso files are committed, so a
# maintainer without goversioninfo still gets an iconned binary, just stamped "dev".
if bash "$ROOT/tools/pack/make-winres.sh" "$VERSION" >/dev/null 2>&1; then
  STAMPED=1
  log "windows resources stamped ${VERSION}"
else
  log "(goversioninfo missing — using the committed resource_windows.syso, version 0.0.0.0)"
fi

log "building revert.exe"
go build -trimpath -ldflags="$LDFLAGS $REVERT_LDFLAGS" -o "$STAGE/revert.exe" ./cmd/revert

log "building vv-padbridge.exe"
go build -trimpath -ldflags="$LDFLAGS" -o "$STAGE/vv-padbridge.exe" ./cmd/vv-padbridge

log "building revert-gui.exe"
( cd gui && go build -trimpath -ldflags="$LDFLAGS" -o "$STAGE/revert-gui.exe" . )

log "building thugkit.exe"
[ -d tools/thugkit ] || die "tools/thugkit missing (the build core repo)"
( cd tools/thugkit && go build -trimpath -ldflags="$LDFLAGS" -o "$STAGE/tools/thugkit/thugkit.exe" ./cmd/thugkit )

if [ -d tools/neverscript ]; then
  log "building ns.exe (skater-extractor helper)"
  ( cd tools/neverscript && go build -trimpath -ldflags="$LDFLAGS" -o "$STAGE/tools/neverscript/ns.exe" ./cmd/ns ) \
    || log "(ns.exe build failed — skater extractor optional; continuing)"
else
  log "(tools/neverscript absent — skipping ns.exe; skater extractor optional)"
fi

# ── stage the shipped tooling assets (NO game data) ─────────────────────────────
log "staging config, mods, and shipped assets"
cp revert.conf "$STAGE/revert.conf"

stage_file() { [ -f "$1" ] && { mkdir -p "$STAGE/$(dirname "$1")"; cp "$1" "$STAGE/$1"; } || true; }
stage_file tools/hudfix/VV.HudFix.asi
stage_file tools/glyphfix/VV.GlyphFix.asi
stage_file tools/keyboardgrid/VV.KeyboardGrid.asi
# ThirteenAG WidescreenFix (open-source; bundled like the Deck lane so only the game data
# is user-supplied — presence-gated, skipped if a maintainer doesn't have it locally).
stage_file tools/TonyHawksUnderground2.WidescreenFix.zip
stage_file tools/controls/thug2-settings.reg
stage_file tools/controls/thug2-settings-windows.reg
stage_file tools/save/tags/VioletVandal.GRF
# DirectInput GUID probe — `revert calibrate-controller` runs it natively to bind pad0.
stage_file tools/xinput-probe/dinput_probe_guid.exe

# mod sources (the .ns + apply scripts thugkit --mods reads). Exclude any blob/ game-
# derived payloads and VCS metadata to stay game-data-free.
if [ -d mods ]; then
  log "staging mods/ sources"
  rsync -a --exclude '.git' --exclude 'blob/' --exclude '*.orig' mods/ "$STAGE/mods/" 2>/dev/null \
    || cp -r mods "$STAGE/mods"
fi

# docs
mkdir -p "$STAGE/docs"
for d in ARCHITECTURE INSTALL BUILD-CONTENTS; do stage_file "docs/$d.md"; done
stage_file README-WINDOWS.txt

# The bundle redistributes third-party components (ThirteenAG's WidescreenFixesPack and the
# Ultimate ASI Loader), so their attribution has to travel *with the bundle*, not just sit
# in the repo. Ship LICENSE alongside it so our own MIT grant is in the box too.
stage_file LICENSE
stage_file THIRD-PARTY-NOTICES.md

# ── zip it ──────────────────────────────────────────────────────────────────────
log "zipping -> $ZIP"
rm -f "$ZIP" "$ZIP.sha256"
( cd "$STAGE" && zip -qr "$ZIP" . )

# Checksum sidecar. `revert update` fetches this alongside the zip and verifies the
# download before unpacking; attach it to the GitHub release next to the zip. Its body is
# plain `sha256sum` format, and the name inside is the asset name (not the dist/ path).
( cd "$OUT" && sha256sum "$(basename "$ZIP")" > "$(basename "$ZIP").sha256" )

log "done:"
ls -lh "$ZIP" "$ZIP.sha256"
log "sha256: $(cut -d' ' -f1 < "$ZIP.sha256")"
log "contents:"; ( cd "$STAGE" && find . -type f | sort | sed 's/^/  /' )
cat <<EOF

To publish this as the asset 'revert update' installs from:
  gh release upload ${VERSION} "$ZIP" "$ZIP.sha256" --repo violetvandal/revert
EOF
