#!/usr/bin/env bash
# encode_bik.sh — encode any audio file to a THUG2-compatible Bink-1 music stream.
#
# Output matches stock THUG2 exactly: BIKi container, 4x4 dummy video,
# binkaudio_dct @ 48000 Hz stereo. (Cracked 2026-06-20; see memory
# project_streaming_mode "ENCODE PIPELINE".)
#
# Pipeline: <input audio> --ffmpeg--> 48k stereo s16 wav --RAD binkc--> .bik
#
# Requires: ffmpeg, GE-Proton wine, and radvideo64.exe (RAD Video Tools, free
# download from radgametools.com; stashed in tools/bink/rad/). binkc opens a
# progress window, so a real/virtual X display is needed (uses DISPLAY=:0).
#
# Usage:  tools/bink/encode_bik.sh <input-audio> <output.bik>
set -euo pipefail

IN="${1:?usage: encode_bik.sh <input-audio> <output.bik>}"
OUT="${2:?usage: encode_bik.sh <input-audio> <output.bik>}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RADEXE="$HERE/rad/radvideo64.exe"
[ -f "$RADEXE" ] || { echo "missing $RADEXE (extract RADTools.7z -> radvideo64.exe)"; exit 1; }

export GE="${GE:-$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64}"
export WINEPREFIX="${WINEPREFIX:-$HOME/.wine-rad}"   # must be win64 (radvideo64 is 64-bit)
export WINEARCH=win64
export WINEDEBUG=-all
export PATH="$GE/bin:$PATH"
export DISPLAY="${DISPLAY:-:0}"

# init the prefix if absent
if [ ! -f "$WINEPREFIX/system.reg" ]; then
  echo ">> initializing win64 wine prefix $WINEPREFIX"
  "$GE/bin/wine" wineboot --init >/dev/null 2>&1 || true
  "$GE/bin/wineserver" -w 2>/dev/null || true
fi

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
WAV="$TMP/src.wav"
echo ">> decoding '$IN' -> 48k/stereo/s16 wav"
ffmpeg -hide_banner -loglevel error -y -i "$IN" -ar 48000 -ac 2 -c:a pcm_s16le "$WAV"

echo ">> binkc -> '$OUT'  (default flags = Bink Audio DCT = stock-match)"
rm -f "$OUT"
: > "$OUT"   # create so winepath can resolve the target, then remove
OUT_ABS="$(cd "$(dirname "$OUT")" && pwd)/$(basename "$OUT")"
rm -f "$OUT"
WIN_IN="$("$GE/bin/wine" winepath -w "$WAV" 2>/dev/null)"
WIN_OUT="$("$GE/bin/wine" winepath -w "$OUT_ABS" 2>/dev/null)"
# binkc needs Windows-form paths (it can't resolve unix-style argv paths).
# /O = auto-overwrite, /# = compress without prompting. NO /L -> DCT (stock).
"$GE/bin/wine" "$RADEXE" binkc "$WIN_IN" "$WIN_OUT" /O /# >/dev/null 2>&1 || true
"$GE/bin/wineserver" -w 2>/dev/null || true

[ -f "$OUT" ] || { echo "!! encode produced no output (is DISPLAY=$DISPLAY reachable?)"; exit 1; }

# verify it's a real Bink-1 file (guards the community '10kb tiny-file' failure)
MAGIC="$(xxd -l4 -p "$OUT")"
SZ="$(stat -c%s "$OUT")"
[ "$MAGIC" = "42494b69" ] || { echo "!! output magic=$MAGIC (expected 42494b69 'BIKi')"; exit 1; }
[ "$SZ" -gt 20000 ] || { echo "!! output suspiciously small ($SZ bytes) — re-encode the source"; exit 1; }
echo ">> OK: $OUT  ($SZ bytes, BIKi)  $(ffprobe -v error -select_streams a:0 -show_entries stream=codec_name,sample_rate,channels -of csv=p=0 "$OUT")"
