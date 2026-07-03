#!/usr/bin/env python3
"""Render the Violet Vandal -> Skatepark Project donation card (640x480).

Drawn in the game's credits style (bold condensed white caps + a violet accent)
to append to the end of the in-game credits movie. Edit the text below freely.

  credits_card.py <out.png>
"""
import sys
from PIL import Image, ImageDraw, ImageFont

W, H = 640, 480
PURPLE, PURPLE_HI, WHITE, GREY = (150, 90, 230), (179, 136, 255), (245, 243, 250), (150, 150, 160)

BLACK_F = ["/usr/share/fonts/google-noto/NotoSans-CondensedBlack.ttf",
           "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf"]
BODY_F  = ["/usr/share/fonts/google-noto/NotoSans-SemiCondensedSemiBold.ttf",
           "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf"]
SMALL_F = ["/usr/share/fonts/google-noto/NotoSans-CondensedLight.ttf",
           "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf"]

# --- the message (edit here) -------------------------------------------------
KICKER  = "VIOLET VANDAL EDITION"
TITLE1  = "SUPPORTS"
TITLE2  = "THE SKATEPARK PROJECT"
BODY    = "If this game brought you joy, or helped you remember a great time in your life, please consider donating."
URL     = "skatepark.org"
DISC1   = "The Skatepark Project is Tony Hawk's 501(c)(3) nonprofit building public skateparks in"
DISC2   = "underserved communities. Not affiliated with or endorsed by this fan project."
# -----------------------------------------------------------------------------


def f(paths, size):
    for c in paths:
        try:
            return ImageFont.truetype(c, size)
        except OSError:
            pass
    return ImageFont.load_default()


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else "card.png"
    im = Image.new("RGB", (W, H), (8, 8, 10))
    d = ImageDraw.Draw(im)

    def ctext(y, s, fnt, fill, track=0):
        tw = sum(d.textlength(c, font=fnt) + track for c in s) - (track if s else 0)
        x = (W - tw) / 2
        for c in s:
            d.text((x, y), c, font=fnt, fill=fill)
            x += d.textlength(c, font=fnt) + track

    def wrap(s, fnt, maxw):
        words, lines, cur = s.split(), [], ""
        for w in words:
            t = (cur + " " + w).strip()
            if d.textlength(t, font=fnt) <= maxw:
                cur = t
            else:
                lines.append(cur); cur = w
        if cur:
            lines.append(cur)
        return lines

    ctext(60, KICKER, f(SMALL_F, 20), PURPLE_HI, track=3)
    ctext(108, TITLE1, f(BLACK_F, 40), WHITE)
    ctext(150, TITLE2, f(BLACK_F, 40), WHITE)
    d.rectangle([W / 2 - 130, 212, W / 2 + 130, 215], fill=PURPLE)
    bf = f(BODY_F, 23); y = 242
    for ln in wrap(BODY, bf, 520):
        ctext(y, ln, bf, WHITE); y += 30
    ctext(y + 14, URL, f(BLACK_F, 34), PURPLE_HI)
    sf = f(SMALL_F, 15)
    ctext(H - 50, DISC1, sf, GREY)
    ctext(H - 32, DISC2, sf, GREY)

    im.save(out)
    print("wrote", out)


if __name__ == "__main__":
    main()
