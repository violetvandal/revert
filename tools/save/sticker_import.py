#!/usr/bin/env python3
"""sticker_import.py — import a custom image as a THUG2 wall-slap sticker.

The Sticker Slap renders the sticker you pick in Create-A-Skater, which is a
CAGR piece (e.g. Graphics\\grap_38). The game loads that piece BOTH loose
(Data/images/CAGR/<cat>/<slot>.img.xbx, used for the in-world slap) and from
cagpieces.prx (used for the CAS editor thumbnail). This replaces both so the
chosen sticker slot becomes your image everywhere.

Workflow: import your image into a slot, then in Create-A-Skater pick that
sticker slot. Any image works (auto-resized to 64x64, palettized).

Usage:
  sticker_import.py <image> --gamedir game-playable-us [--slot grap_38]
  sticker_import.py <image> --gamedir game-playable-us --slot grap_50 --colors 256
"""
import sys, os, argparse, glob
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(HERE, "..", "prx"))
import png2img
import prx


def find_loose(gamedir, slot):
    hits = glob.glob(os.path.join(gamedir, "Data", "images", "CAGR", "*", slot + ".img.xbx"))
    return hits[0] if hits else None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("image")
    ap.add_argument("--gamedir", required=True)
    ap.add_argument("--slot", default="grap_38",
                    help="CAGR piece to overwrite (the sticker slot you'll select in CAS). Default grap_38.")
    ap.add_argument("--colors", type=int, default=256)
    a = ap.parse_args()

    loose = find_loose(a.gamedir, a.slot)
    if not loose:
        sys.exit("slot %s not found under %s/Data/images/CAGR/*/" % (a.slot, a.gamedir))
    cat = os.path.basename(os.path.dirname(loose))            # e.g. Graphics
    entry = "images\\CAGR\\%s\\%s.img.xbx" % (cat, a.slot)    # archive entry name

    data = png2img.encode(a.image, size=(64, 64), colors=a.colors)
    if data[:4] != bytes([2, 0, 0, 0]):
        sys.exit("encode produced unexpected header")

    # 1) loose file (drives the in-world slap)
    if not os.path.exists(loose + ".orig"):
        os.replace(loose, loose + ".orig") if False else __import__("shutil").copyfile(loose, loose + ".orig")
    open(loose, "wb").write(data)
    print("wrote loose %s (%d bytes)" % (os.path.relpath(loose, a.gamedir), len(data)))

    # 2) cagpieces.prx entry (drives the CAS editor thumbnail) — via proven prx.py replacez
    prxpath = os.path.join(a.gamedir, "Data", "pre", "cagpieces.prx")
    ver, es = prx.parse(open(prxpath, "rb").read())
    if not prx.find(es, entry):
        print("  (warning: %s not in cagpieces.prx — editor thumbnail unchanged)" % entry)
    else:
        import tempfile, subprocess
        with tempfile.NamedTemporaryFile(suffix=".img.xbx", delete=False) as tf:
            tf.write(data); tmp = tf.name
        subprocess.run([sys.executable, os.path.join(HERE, "..", "prx", "prx.py"),
                        "replacez", prxpath, entry, tmp, prxpath], check=True)
        os.unlink(tmp)
        print("injected into cagpieces.prx :: %s" % entry)

    print("done. In Create-A-Skater, pick sticker slot '%s'." % a.slot)


if __name__ == "__main__":
    main()
