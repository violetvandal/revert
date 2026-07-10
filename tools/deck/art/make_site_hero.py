#!/usr/bin/env python3
"""Composite the thug2vandal.com homepage hero: the in-game Violet Vandal render
under the transparent THUG2 title logo + the "VIOLET VANDAL EDITION" banner.

Reuses the brand kit from make_art.py (PURPLE banner, fonts). Deterministic and
re-runnable — regenerate whenever the base render or the logo changes.

  usage:  make_site_hero.py <base_render.jpg> [out.jpg] [logo_width_frac]
  default logo_width_frac = 0.64 (the shipped "Option B" — smaller logo, more skater)

The base render is the site's public/img/vv-hero.jpg; the logo is base/logo.png here.
"""
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import make_art as M  # noqa: E402  (PURPLE banner + fonts)
from PIL import Image  # noqa: E402

DEFAULT_LOGO = os.path.join(HERE, "logo.png")


def build(base_path, out_path, logo_width=0.64, logo_path=DEFAULT_LOGO):
    hero = Image.open(base_path).convert("RGBA")
    W, H = hero.size

    # Dark gradient over the lower half to seat the logo (transparent up top,
    # deepening toward the bottom where the title sits).
    grad = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    px = grad.load()
    start, max_a = 0.50, 190
    for y in range(H):
        t = max(0.0, min(1.0, (y / H - start) / (1 - start)))
        a = int(max_a * t ** 1.3)
        for x in range(W):
            px[x, y] = (6, 4, 12, a)
    hero = Image.alpha_composite(hero, grad)

    # Title logo, fit to width, seated just above the edition banner.
    logo = Image.open(logo_path).convert("RGBA")
    tw = int(W * logo_width)
    logo = logo.resize((tw, round(logo.height * tw / logo.width)), Image.LANCZOS)
    banner_h = int(H * 0.062)
    ly = H - banner_h - logo.height - int(H * 0.015)
    hero.alpha_composite(logo, ((W - tw) // 2, ly))

    # Edition banner (the same brand banner as the Steam cover) at the bottom.
    final = M.banner(hero, height_frac=0.062, fsz=34, track=3).convert("RGB")
    final.save(out_path, quality=88)
    print(f"wrote {out_path}  {final.size}")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        raise SystemExit(__doc__)
    base = sys.argv[1]
    out = sys.argv[2] if len(sys.argv) > 2 else base
    frac = float(sys.argv[3]) if len(sys.argv) > 3 else 0.64
    build(base, out, frac)
