#!/usr/bin/env bash
#
# sync-to-deck.sh — push the built "THUG2: Violet Vandal Edition" + the Revert
# toolkit from this dev machine to a Steam Deck. Runs HERE (the source), over SSH.
#
#   tools/deck/sync-to-deck.sh [SRC] [DEST]
#     SRC   default: this repo root (the dir containing revert.conf)
#     DEST  default: deck@192.168.1.204:thug2
#
# WHITELIST, not blacklist: the dev machine's repo also holds many GB of disc
# rips/backups/build assets with unpredictable names (a blacklist missed them and
# tried to push ~12GB). The Deck only needs what SETUP + the qol RUN lane use — the
# game is already built into game-playable-us. So we copy exactly that set.
#
# Wipe the Deck's ~/thug2 first for a clean tree:  ssh deck rm -rf '~/thug2'
set -euo pipefail

SRC="${1:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
DEST="${2:-deck@192.168.1.204:thug2}"
cd "$SRC"
[[ -f revert.conf ]] || { echo "no revert.conf in SRC=$SRC" >&2; exit 1; }
[[ -d game-playable-us ]] || echo "warning: game-playable-us missing — run 'revert build' first" >&2

# Only what the Deck needs. tools/bink (soundtrack) + tools/save (CAS) + tools/thugkit
# (build engine) are dev/build-only — the qol lane degrades gracefully without them.
ITEMS=(
  revert revert.conf README.md
  share docs
  game-playable-us
  tools/controls tools/trigger-bridge tools/xinput-probe
  tools/glyphfix tools/hudfix tools/deck
  tools/wine-11.11-staging-amd64.tar.xz
  tools/dxvk-2.7.1.tar.gz
)

# -R keeps each item's relative path under DEST (tools/controls -> DEST/tools/controls).
exec rsync -aR --delete-after --mkpath --info=progress2 "${ITEMS[@]}" "$DEST/"
