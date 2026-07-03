#!/usr/bin/env python3
"""apply_tags.py — install ALL custom Create-A-Graphic tags, turnkey.

Closes the rebuild gap: a custom tag has two halves outside the .ns/.qb mod
pipeline — the `.GRF` file in Save/ and (for custom-image tags) an injected
sprite in Data/pre/cagpieces.prx. A rebuild resets Data/pre from pristine, so
the sprite injection is lost; a *fresh* build also starts with an empty Save/,
so even stock-sprite tags vanish. This re-applies both from a tracked source
folder so custom tags always survive a rebuild.

Drop files into tools/save/tags/ :
  * a .GRF file   -> copied verbatim into the install's Save/ (preserves an
                     in-game-authored tag, e.g. the VioletVandal persona tag,
                     which composes stock CAGR sprites + text — fully
                     reproducible, no cagpieces work needed).
  * an image      -> built into a from-scratch full-canvas custom-image tag:
                     the sprite is injected into cagpieces.prx (slots grap_50,
                     grap_51, ... — clear of the grap_1.. range apply_stickers
                     uses) and a <name>.GRF (named after the file) is written
                     to Save/. The image is the reproducible source.

Idempotent; re-applied by rebuild-playable.sh (apply_cas_asset_mods).

Usage:  apply_tags.py <gamedir>
"""
import sys, os, glob, shutil, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
TAGS = os.path.join(HERE, "tags")
IMG_EXTS = (".png", ".jpg", ".jpeg", ".bmp", ".gif", ".tga", ".webp")
TAG_SLOT_BASE = 50          # grap_50, grap_51, ... (stickers own grap_1..)


def save_dir(gamedir):
    for sub in ("Save", "Data/Game/Save", "data/game/save"):
        p = os.path.join(gamedir, sub)
        if os.path.isdir(p):
            return p
    p = os.path.join(gamedir, "Save")          # default; created on rebuild
    os.makedirs(p, exist_ok=True)
    return p


def main():
    if len(sys.argv) < 2:
        print(__doc__); sys.exit(2)
    gamedir = sys.argv[1]
    if not os.path.isdir(TAGS):
        print("no tags dir (%s) — nothing to apply" % TAGS); return
    files = sorted(glob.glob(os.path.join(TAGS, "*")))
    grfs = [f for f in files if f.lower().endswith(".grf")]
    imgs = [f for f in files if f.lower().endswith(IMG_EXTS)]
    if not grfs and not imgs:
        print("no .GRF or images in %s — nothing to apply" % TAGS); return

    sd = save_dir(gamedir)
    # 1. stock-sprite / in-game-authored tags: copy the .GRF straight into Save/
    for g in grfs:
        dest = os.path.join(sd, os.path.basename(g))
        shutil.copy(g, dest)
        print("  %-32s -> Save/%s" % (os.path.basename(g), os.path.basename(g)))

    # 2. custom-image tags: build .GRF + inject sprite into cagpieces.prx
    if imgs:
        import thug2_tag_importer as tagimp
        with tempfile.TemporaryDirectory() as tmp:
            for i, img in enumerate(imgs):
                slot = "grap_%d" % (TAG_SLOT_BASE + i)
                info = tagimp.run(img, gamedir, name=None, slot=slot,
                                  outdir=tmp, preview=False, install=True)
                print("  %-32s -> %s.GRF (sprite %s)"
                      % (os.path.basename(img), info["name"], slot))

    print("applied %d tag .GRF + %d custom-image tag(s)" % (len(grfs), len(imgs)))


if __name__ == "__main__":
    main()
