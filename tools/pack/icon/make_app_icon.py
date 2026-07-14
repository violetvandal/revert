#!/usr/bin/env python3
"""The Vv monogram, drawn in a clean condensed-black face (no game assets).

The V and the v are drawn separately on a shared baseline rather than as the string "Vv",
so the gap between them and the size of the lowercase v are both tunable. Default kerning
leaves a slack gap between a cap V and a lowercase v, and the v's natural x-height reads
small once the pair is shrunk into a 16px taskbar icon.
"""
import os
import sys
from PIL import Image, ImageDraw, ImageFont

PURPLE_HI = (179, 136, 255)
TEXT = (245, 240, 255)
S = 1024
ICO_SIZES = [16, 24, 32, 48, 64, 128, 256]

FONTS = [
    "/usr/share/fonts/google-noto/NotoSans-CondensedBlack.ttf",
    "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf",
]


def font(sz):
    for p in FONTS:
        if os.path.exists(p):
            return ImageFont.truetype(p, sz)
    return ImageFont.load_default()


def tile():
    im = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    g = Image.new("RGBA", (1, S))
    gd = ImageDraw.Draw(g)
    for y in range(S):
        t = y / (S - 1)
        c = tuple(int(a + (b - a) * t) for a, b in zip((124, 40, 175), (74, 16, 112)))
        gd.point((0, y), fill=(*c, 255))
    g = g.resize((S, S))
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=int(S * 0.22), fill=255)
    im.paste(g, (0, 0), mask)
    ImageDraw.Draw(im).rounded_rectangle(
        [0, 0, S - 1, S - 1], radius=int(S * 0.22), outline=(*PURPLE_HI, 90), width=int(S * 0.012))
    return im


def mark(cap_px, v_scale=1.0, gap_px=0):
    """V and v on a shared baseline. Returns an L mask, tightly cropped."""
    W = H = S * 2
    m = Image.new("L", (W, H), 0)
    d = ImageDraw.Draw(m)
    fV = font(cap_px)
    fv = font(int(cap_px * v_scale))
    base_x, base_y = W // 4, H // 2
    d.text((base_x, base_y), "V", font=fV, fill=255, anchor="ls")
    adv = d.textlength("V", font=fV)
    d.text((base_x + adv + gap_px, base_y), "v", font=fv, fill=255, anchor="ls")
    return m.crop(m.getbbox())


def compose(mk, underline=True, width_frac=0.64):
    im = tile()
    w, h = mk.size
    tw = int(S * width_frac)
    th = int(h * tw / w)
    mk = mk.resize((tw, th), Image.LANCZOS)
    y = int(S * (0.46 if underline else 0.50) - th / 2)
    layer = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    layer.paste(Image.new("RGBA", mk.size, (*TEXT, 255)), ((S - tw) // 2, y), mk)
    im = Image.alpha_composite(im, layer)
    if underline:
        ImageDraw.Draw(im).rounded_rectangle(
            [S * 0.30, S * 0.745, S * 0.70, S * 0.782], radius=S * 0.019, fill=(*PURPLE_HI, 255))
    return im


# The font's own kerning and x-height. Tightening the gap, or enlarging the lowercase v,
# makes the two letterforms collide: at 16px the pair then reads as a single "W".
V_SCALE, GAP_PX, WIDTH_FRAC = 1.00, 0, 0.64


def build():
    return compose(mark(int(S * 0.46), V_SCALE, GAP_PX), width_frac=WIDTH_FRAC)


if __name__ == "__main__":
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.dirname(os.path.abspath(__file__))
    im = build()
    im.save(os.path.join(out, "revert-512.png"))
    im.save(os.path.join(out, "revert.ico"), format="ICO", sizes=[(s, s) for s in ICO_SIZES])
    # macOS .app-bundle icon. PIL writes a full multi-size .icns from a 1024 base; the
    # macOS lane copies this into each bundle's Contents/Resources (see internal/core/mac.go).
    im.resize((1024, 1024), Image.LANCZOS).save(os.path.join(out, "revert.icns"), format="ICNS")
    print("wrote revert.ico + revert.icns + revert-512.png ->", out)
