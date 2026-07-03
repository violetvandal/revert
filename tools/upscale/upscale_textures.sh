#!/usr/bin/env bash
# AI-upscale the skater's textures 4x with Real-ESRGAN (ncnn) then they feed the render.
# Usage: upscale_textures.sh [model]   (model default: realesr-animevideov3-x4)
set -e
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$HERE/realesrgan-ncnn-vulkan-v0.2.0-ubuntu/realesrgan-ncnn-vulkan"
MODELS="$HERE/realesrgan-ncnn-vulkan-v0.2.0-ubuntu/models"
TEX="$(cd "$HERE/.." && pwd)/save/renders/tex"
MODEL="${1:-realesr-animevideov3-x4}"
"$BIN" -i "$TEX" -o "$TEX" -m "$MODELS" -n "$MODEL" -s 4
echo "upscaled $TEX with $MODEL"
