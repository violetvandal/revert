#!/usr/bin/env python3
"""apply_stickers.py — install ALL custom wall-slap stickers, turnkey.

Drop image files into tools/save/stickers/ and they become in-game stickers.
No slot/manifest management: images (sorted by filename) are auto-assigned to
the first Graphics CAGR slots (grap_1, grap_2, ...) so they appear at the TOP
of the Create-A-Skater sticker picker. Each overwrites that slot's loose CAGR
piece + cagpieces.prx entry (via sticker_import.py). Idempotent; re-applied by
rebuild-playable.sh so custom stickers always survive a rebuild.

In game: Create-A-Skater -> sticker picker -> your images are the first
Graphics entries.

Usage:  apply_stickers.py <gamedir>
"""
import sys, os, glob, subprocess

HERE = os.path.dirname(os.path.abspath(__file__))
STK = os.path.join(HERE, "stickers")
EXTS = (".png", ".jpg", ".jpeg", ".bmp", ".gif", ".tga", ".webp")


def main():
    if len(sys.argv) < 2:
        print(__doc__); sys.exit(2)
    gamedir = sys.argv[1]
    if not os.path.isdir(STK):
        print("no stickers dir (%s) — nothing to apply" % STK); return
    imgs = sorted(p for p in glob.glob(os.path.join(STK, "*"))
                  if p.lower().endswith(EXTS))
    if not imgs:
        print("no images in %s — nothing to apply" % STK); return
    for i, img in enumerate(imgs, start=1):
        slot = "grap_%d" % i
        subprocess.run([sys.executable, os.path.join(HERE, "sticker_import.py"),
                        img, "--gamedir", gamedir, "--slot", slot], check=True)
        print("  %-28s -> Graphics slot %d (%s)" % (os.path.basename(img), i, slot))
    print("applied %d custom sticker(s) — pick them at the top of the CAS sticker list" % len(imgs))


if __name__ == "__main__":
    main()
