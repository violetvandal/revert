#!/usr/bin/env bash
# encode_video_bik.sh — encode any video to a THUG2-compatible Bink VIDEO movie.
#
# Output matches stock THUG2 movies: BIKi, 640x480 binkvideo @ 29.97 fps,
# binkaudio_dct 48000 Hz stereo. Source is letterboxed into 640x480 (aspect kept).
#
# RAD's binkc can't open AVIs under Wine (no VFW), so we feed it a numbered TGA
# image sequence (its own loader), then mux audio with BinkMix:
#   frames --binkc /F29.97--> video.bik  --BinkMix +wav--> out.bik
# (Bink VIDEO encoding cracked 2026-06-29; audio pipeline from encode_bik.sh.)
#
#   encode_video_bik.sh [--card PNG [--card-secs N]] <input-video> <output.bik>
set -euo pipefail

CARD="" CARD_SECS=8
while [[ "${1:-}" == --* ]]; do
  case "$1" in
    --card)      CARD="$2"; shift 2;;
    --card-secs) CARD_SECS="$2"; shift 2;;
    *) echo "unknown flag $1" >&2; exit 2;;
  esac
done
IN="${1:?usage: encode_video_bik.sh [--card PNG] <input-video> <output.bik>}"
OUT="${2:?usage: encode_video_bik.sh [--card PNG] <input-video> <output.bik>}"
[[ -f "$IN" ]] || { echo "no such input: $IN" >&2; exit 1; }

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RADEXE="$HERE/rad/radvideo64.exe"
[[ -f "$RADEXE" ]] || { echo "missing $RADEXE" >&2; exit 1; }

export GE="${GE:-$HOME/.local/share/lutris/runners/wine/wine-ge-8-26-x86_64}"
export WINEPREFIX="${WINEPREFIX:-$HOME/.wine-rad}" WINEARCH=win64 WINEDEBUG=-all
export PATH="$GE/bin:$PATH" DISPLAY="${DISPLAY:-:0}"
[[ -f "$WINEPREFIX/system.reg" ]] || { wine wineboot --init >/dev/null 2>&1 || true; wineserver -w 2>/dev/null || true; }

# `wine <gui app>` can return BEFORE the app finishes — binkc/BinkMix run their compressor
# window asynchronously. Wait for the RAD process to actually exit before using its output
# (otherwise we'd race a partial .bik — this bit us with a 1-frame false success).
wait_rad(){ local i; for i in $(seq 1 1800); do pgrep -f radvideo64 >/dev/null || { sleep 1; return 0; }; sleep 1; done; }

# binkc reads its sequence + writes from the wine drive; work under drive_c so Wine
# has no path/permission surprises (raw /tmp under Z: has tripped read errors).
WORK="$WINEPREFIX/drive_c/vvenc.$$"; SEQ="$WORK/seq"
mkdir -p "$SEQ"
cleanup(){ rm -rf "$WORK"; }; trap cleanup EXIT

echo ">> [1/4] frames -> $SEQ (640x480, letterboxed, 30fps)"
LETTERBOX="scale=640:480:force_original_aspect_ratio=decrease,pad=640:480:(640-iw)/2:(480-ih)/2,setsar=1,fps=30"
if [[ -n "$CARD" ]]; then
  [[ -f "$CARD" ]] || { echo "no such card: $CARD" >&2; exit 1; }
  FO=$(awk "BEGIN{print $CARD_SECS-1}")
  ffmpeg -hide_banner -loglevel error -y -i "$IN" -loop 1 -t "$CARD_SECS" -i "$CARD" \
    -filter_complex "[0:v]$LETTERBOX[v0];[1:v]scale=640:480,setsar=1,fps=30,fade=t=in:st=0:d=0.5,fade=t=out:st=$FO:d=1[v1];[v0][v1]concat=n=2:v=1:a=0[v]" \
    -map "[v]" -pix_fmt bgr24 "$SEQ/fr%04d.tga"
  echo ">> [2/4] audio (+ ${CARD_SECS}s silence under the card) -> wav"
  ffmpeg -hide_banner -loglevel error -y -i "$IN" -f lavfi -t "$CARD_SECS" -i anullsrc=r=48000:cl=stereo \
    -filter_complex "[0:a]aresample=48000,aformat=channel_layouts=stereo[a0];[a0][1:a]concat=n=2:v=0:a=1[a]" \
    -map "[a]" -c:a pcm_s16le "$WORK/aud.wav"
else
  ffmpeg -hide_banner -loglevel error -y -i "$IN" -vf "$LETTERBOX" -pix_fmt bgr24 "$SEQ/fr%04d.tga"
  echo ">> [2/4] audio -> wav"
  ffmpeg -hide_banner -loglevel error -y -i "$IN" -ar 48000 -ac 2 -c:a pcm_s16le "$WORK/aud.wav"
fi
NF=$(ls "$SEQ" | wc -l); echo "   $NF frames"

# RAD pops a "Treat as sequence?" dialog if given just the first frame (blocks headless).
# An explicit .LST list of every frame is read directly with no prompt (cracked 2026-06-29).
echo ">> [3/4] binkc (.LST of $NF frames) -> video.bik @ /F29.97"
WB="$(basename "$WORK")"
for fr in "$SEQ"/fr*.tga; do printf 'C:\\%s\\seq\\%s\r\n' "$WB" "${fr##*/}"; done > "$WORK/seq.lst"
wine "$RADEXE" binkc 'C:\'"$WB"'\seq.lst' 'C:\'"$WB"'\video.bik' /F29.97 /O /# >/dev/null 2>&1 || true
wait_rad
[[ -f "$WORK/video.bik" ]] || { echo "!! binkc produced no video (a RAD dialog likely blocked it; DISPLAY=$DISPLAY)" >&2; exit 1; }

echo ">> [4/4] BinkMix audio -> $OUT"
wine "$RADEXE" BinkMix 'C:\'"$(basename "$WORK")"'\video.bik' 'C:\'"$(basename "$WORK")"'\aud.wav' 'C:\'"$(basename "$WORK")"'\final.bik' /O /# >/dev/null 2>&1 || true
wait_rad
[[ -f "$WORK/final.bik" ]] || { echo "!! BinkMix produced no output" >&2; exit 1; }

mkdir -p "$(dirname "$OUT")"; cp "$WORK/final.bik" "$OUT"
MAGIC=$(xxd -l4 -p "$OUT"); SZ=$(stat -c%s "$OUT")
[[ "$MAGIC" == "42494b69" ]] || { echo "!! bad magic $MAGIC (expected BIKi)" >&2; exit 1; }
echo ">> OK: $OUT  ($SZ bytes)  $(ffprobe -v error -show_entries stream=codec_name,width,height,r_frame_rate -of csv=p=0 "$OUT" 2>/dev/null | tr '\n' ' ')"
