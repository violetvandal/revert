#!/usr/bin/env python3
"""Composite the "VV Edition" Steam library artwork from base images.

Reads base/{cover,hero,logo}.png (whatever is present) and writes the final, marked
cover/header/hero/logo/icon.png next to it — exactly the files `revert setup` installs
into Steam's userdata grid/ folder. Re-runnable and deterministic.

  cover  : portrait tile        600x900   (required base)  -> bottom "edition" banner
  hero   : banner behind Play   3840x1240 (optional base)  -> small corner pill
  header : landscape capsule    460x215   (from hero base) -> small corner pill
  logo   : transparent wordmark 1280x720  (optional base)  -> fitted, untouched
  icon   : small icon           256x256   (from logo, else cover logo band)
"""
import os
from PIL import Image, ImageDraw, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
BASE = os.path.join(HERE, "base")

PURPLE    = (106, 27, 154)    # deep violet banner
PURPLE_HI = (179, 136, 255)   # bright accent line
TEXT      = (245, 240, 255)
TAG       = "VIOLET VANDAL EDITION"

FONTS = [
    "/usr/share/fonts/google-noto/NotoSans-CondensedBlack.ttf",
    "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf",
    "/usr/share/fonts/liberation-sans/LiberationSans-Bold.ttf",
]


def font(sz):
    for p in FONTS:
        if os.path.exists(p):
            return ImageFont.truetype(p, sz)
    return ImageFont.load_default()


def _tag_width(draw, text, fnt, track):
    return sum(draw.textlength(c, font=fnt) + track for c in text) - track


def _draw_tracked(draw, xy, text, fnt, fill, track):
    x, y = xy
    for ch in text:
        draw.text((x, y), ch, font=fnt, fill=fill)
        x += draw.textlength(ch, font=fnt) + track


def fill_crop(im, w, h):
    """Scale to COVER w x h, then center-crop."""
    im = im.convert("RGB")
    s = max(w / im.width, h / im.height)
    im = im.resize((round(im.width * s), round(im.height * s)), Image.LANCZOS)
    x, y = (im.width - w) // 2, (im.height - h) // 2
    return im.crop((x, y, x + w, y + h))


def _autofit(probe, max_w, fsz, track):
    f = font(fsz)
    while fsz > 8 and _tag_width(probe, TAG, f, track) > max_w:
        fsz -= 1
        f = font(fsz)
    return f, fsz


def banner(im, height_frac=0.075, fsz=30, track=2):
    """Bottom full-width edition banner (the cover)."""
    im = im.convert("RGBA")
    W, H = im.size
    bh = int(H * height_frac)
    bar = Image.new("RGBA", (W, bh), (0, 0, 0, 0))
    bd = ImageDraw.Draw(bar)
    bd.rectangle([0, 0, W, bh], fill=(*PURPLE, 235))
    bd.rectangle([0, 0, W, max(2, bh // 20)], fill=(*PURPLE_HI, 255))
    f, fsz = _autofit(bd, W * 0.90, fsz, track)
    tw = _tag_width(bd, TAG, f, track)
    _draw_tracked(bd, ((W - tw) // 2, (bh - fsz) // 2 - int(fsz * 0.18)),
                  TAG, f, (*TEXT, 255), track)
    im.alpha_composite(bar, (0, H - bh))
    return im.convert("RGB")


def pill(im, fsz=44, track=3, pad=None, margin=None):
    """Small rounded edition pill, bottom-left (hero / header)."""
    im = im.convert("RGBA")
    W, H = im.size
    pad = pad if pad is not None else int(fsz * 0.55)
    margin = margin if margin is not None else int(H * 0.05)
    d = ImageDraw.Draw(im)
    f, fsz = _autofit(d, W * 0.55, fsz, track)
    tw = _tag_width(d, TAG, f, track)
    pw, ph = int(tw + 2 * pad), int(fsz + 2 * pad)
    p = Image.new("RGBA", (pw, ph), (0, 0, 0, 0))
    pd = ImageDraw.Draw(p)
    pd.rounded_rectangle([0, 0, pw - 1, ph - 1], radius=ph // 2, fill=(*PURPLE, 235))
    pd.rounded_rectangle([0, 0, pw - 1, ph - 1], radius=ph // 2, outline=(*PURPLE_HI, 255),
                         width=max(2, ph // 22))
    _draw_tracked(pd, (pad, pad - int(fsz * 0.18)), TAG, f, (*TEXT, 255), track)
    im.alpha_composite(p, (margin, H - ph - margin))
    return im.convert("RGB")


def fit_transparent(im, w, h):
    """Fit (contain) onto a transparent w x h canvas, centered."""
    im = im.convert("RGBA")
    s = min(w / im.width, h / im.height)
    im = im.resize((max(1, round(im.width * s)), max(1, round(im.height * s))), Image.LANCZOS)
    canvas = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    canvas.alpha_composite(im, ((w - im.width) // 2, (h - im.height) // 2))
    return canvas


def out(name):
    return os.path.join(HERE, name)


def main():
    made = []

    cover_b = os.path.join(BASE, "cover.png")
    if not os.path.exists(cover_b):
        raise SystemExit(f"need a base cover at {cover_b}")
    cover = fill_crop(Image.open(cover_b), 600, 900)
    banner(cover).save(out("cover.png"));  made.append("cover.png")

    # icon: prefer the transparent logo; else crop the cover's wordmark band.
    logo_b = os.path.join(BASE, "logo.png")
    if os.path.exists(logo_b):
        icon = fit_transparent(Image.open(logo_b), 256, 256)
    else:
        band = cover.crop((0, int(900 * 0.22), 600, int(900 * 0.52)))  # the black logo band
        icon = Image.new("RGBA", (256, 256), (10, 10, 12, 255))
        b = band.convert("RGBA"); s = 256 / b.width
        b = b.resize((256, max(1, round(b.height * s))), Image.LANCZOS)
        icon.alpha_composite(b, (0, (256 - b.height) // 2))
    icon.save(out("icon.png"));  made.append("icon.png")

    hero_b = os.path.join(BASE, "hero.png")
    if os.path.exists(hero_b):
        h = Image.open(hero_b)
        pill(fill_crop(h, 3840, 1240), fsz=46).save(out("hero.png"));   made.append("hero.png")
        pill(fill_crop(h, 460, 215), fsz=15, track=1).save(out("header.png")); made.append("header.png")

    if os.path.exists(logo_b):
        fit_transparent(Image.open(logo_b), 1280, 720).save(out("logo.png"));  made.append("logo.png")

    print("wrote: " + ", ".join(made))
    if not os.path.exists(hero_b):
        print("note: no base/hero.png -> hero/header skipped")
    if not os.path.exists(logo_b):
        print("note: no base/logo.png -> logo skipped (icon derived from cover)")


if __name__ == "__main__":
    main()
