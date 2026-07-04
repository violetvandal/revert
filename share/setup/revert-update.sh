#!/usr/bin/env bash
#
# revert-update.sh — update the Revert toolkit to the latest tagged release and
# rebuild. The install is a `git clone --recursive`, and each release tag pins the
# exact thugkit + revert-mods (+ nested NeverScript) commits, so an update is a
# fetch → checkout tag → submodule sync → rebuild. Game data is never touched.
#
#   revert update [--check] [--force]
#     --check   report whether a newer release exists, then exit (no changes)
#     --force   proceed even if the working tree has local (tracked) changes
#
# Put machine-specific config in revert.conf.local (gitignored) so it survives updates.
#
set -euo pipefail

REVERT_ROOT="${REVERT_ROOT:-$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)}"
export REVERT_ROOT
# shellcheck disable=SC1090
source "${REVERT_ROOT}/revert.conf"

log()  { printf '\033[1;34m[update]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[update:warn]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m[update:error]\033[0m %s\n' "$*" >&2; exit 1; }
note() { printf '\033[0;36m[update]\033[0m %s\n' "$*"; }

CHECK=0; FORCE=0
for a in "$@"; do case "$a" in
  --check) CHECK=1;;
  --force) FORCE=1;;
  *) err "unknown option '$a' (use: --check | --force)";;
esac; done

[[ -d "${REVERT_ROOT}/.git" ]] || err "not a git checkout — updates need a 'git clone --recursive' install"
command -v git >/dev/null || err "git required"
cd "$REVERT_ROOT"

# Updates pull from the public release repo's 'origin'. A local/development checkout
# (e.g. the private dev root) has no remote — distinguish that from a real network
# failure so the message is actionable, not a scary "check your network".
if ! git remote get-url origin >/dev/null 2>&1; then
  err "no 'origin' remote — this looks like a local/development checkout. 'revert update' updates installs made with: git clone --recursive https://github.com/violetvandal/revert  (for a dev checkout, pull/build manually)."
fi

log "fetching releases ..."
git fetch --tags --quiet origin || err "git fetch from origin failed — check your network (or 'git fetch origin' by hand for the real error)."

current="$(git describe --tags --always 2>/dev/null || echo unknown)"
latest="$(git tag -l 'v*' --sort=-v:refname | head -1)"
[[ -n "$latest" ]] || err "no release tags found on origin"

tgt_commit="$(git rev-parse "${latest}^{commit}")"
# Up to date when HEAD already CONTAINS the latest release — i.e. HEAD is at the tag
# or ahead of it (e.g. a dev build tracking main). A plain != check would wrongly
# offer to "update" (downgrade) anyone ahead of the newest tag.
if git merge-base --is-ancestor "$tgt_commit" HEAD 2>/dev/null; then
  log "already up to date (${current})"
  exit 0
fi

log "current: ${current}   latest release: ${latest}"
if [[ "$CHECK" == 1 ]]; then
  note "update available -> run: revert update"
  exit 0
fi

# Preserve local edits to TRACKED files (config belongs in revert.conf.local, which
# is gitignored and untouched by the checkout).
stashed=0
if ! git diff --quiet HEAD -- 2>/dev/null; then
  [[ "$FORCE" == 1 ]] || err "working tree has local changes to tracked files. Move config into revert.conf.local, commit/stash your edits, or re-run with --force."
  log "stashing local changes (restored after update)"
  git stash push -u -m "revert-update autostash" >/dev/null 2>&1 && stashed=1
fi

log "updating to ${latest} ..."
git checkout -q "$latest" || err "checkout ${latest} failed"
log "syncing components (thugkit + mods + neverscript) ..."
git submodule sync --recursive --quiet
git submodule update --init --recursive || err "submodule update failed"

if [[ "$stashed" == 1 ]]; then
  git stash pop >/dev/null 2>&1 || warn "your stashed changes conflicted — resolve manually (see: git stash list)"
fi

# Force a thugkit rebuild — the old binary is stale after the submodule bump.
if command -v go >/dev/null; then
  log "rebuilding thugkit ..."
  ( cd "$(dirname "$THUGKIT")" && go build -o "$(basename "$THUGKIT")" ./cmd/thugkit ) || err "thugkit build failed"
else
  warn "no go toolchain — thugkit not rebuilt; install Go then run: revert build"
fi

# Rebuild the edition (idempotent; preserves your Save/) if the game base is present.
if [[ -d "${PRISTINE_DIR}/Data/pre" ]]; then
  log "rebuilding the edition ..."
  "${REVERT_ROOT}/revert" build || err "revert build failed"
else
  note "no pristine base yet — run: revert acquire-game-data, then: revert build"
fi

log "updated to $(git describe --tags --always). Done."
