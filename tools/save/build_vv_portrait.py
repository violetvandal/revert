#!/usr/bin/env python3
# build_vv_portrait.py — regenerate the vv-skater Select-Skater portrait ss_vv.img.xbx.
#
# THUG2's Select Skater screen draws each skater's `select_icon` as a small 32x64
# sprite. The stock portraits (Data/images/MainmenuSprites/ss_*.img.xbx) are all
# WHITE full-body silhouettes (the shape lives in the alpha channel; every opaque
# pixel is pure white), stored vertically flipped. This tool renders Violet Vandal
# from her save, reduces her to that exact style, and writes a byte-format-matching
# ss_vv.img.xbx (32x64, 8bpp, 16-colour palette, Xbox-swizzled, flipped) so she gets
# a native-looking portrait instead of reusing ss_custom.
#
# It is AUTHOR-SIDE (needs Blender + the extractor + her .SKA). The committed asset
# mods/src/vv-skater/assets/ss_vv.img.xbx is what the build ships; run this only to
# regenerate it (e.g. after changing her appearance).
#
#   python3 tools/save/build_vv_portrait.py [--ska <save.SKA>] [--gamedir <dir>]
#                                           [--pose relaxed] [--skip-render] [--out <file>]
import os, sys, subprocess, argparse, struct
import numpy as np
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, '..', '..'))
sys.path.insert(0, HERE)
import png2img  # noqa: E402  (the PNG -> .img.xbx encoder; matches the stock header exactly)

RENDER = os.path.join(ROOT, 'thug2-skater-extractor', 'skaterkit', 'render_skater.py')
DEF_SKA = os.path.join(ROOT, 'game-playable-us', 'Save', 'Violet Vandal.SKA')
DEF_GAME = os.path.join(ROOT, 'game-playable-us')
DEF_OUT = os.path.join(ROOT, 'mods', 'src', 'vv-skater', 'assets', 'ss_vv.img.xbx')
W, H, NCOL = 32, 64, 16   # stock portrait geometry: 32x64, 16-colour palette (=> 2144 bytes)


def render_raw(ska, gamedir, pose, outdir):
    """Run the extractor and return the path to its alpha cutout (_render_raw.png)."""
    os.makedirs(outdir, exist_ok=True)
    subprocess.run([sys.executable, RENDER, ska, gamedir,
                    '--frame', 'body', '--pose', pose, '--res', '640',
                    '--out', os.path.join(outdir, '_vvport_composed.png')], check=True)
    return os.path.join(outdir, '_render_raw.png')


def silhouette(raw_png):
    """Alpha cutout -> a 32x64 pure-white silhouette (RGB=255, alpha=shape), head-up."""
    raw = Image.open(raw_png).convert('RGBA')
    a = np.array(raw.getchannel('A'))
    ys, xs = np.where(a > 32)
    fig = raw.crop((xs.min(), ys.min(), xs.max() + 1, ys.max() + 1))
    # fit inside the frame with a small margin (the stock silhouettes nearly fill it)
    mw, mh = int(W * 0.96), int(H * 0.96)
    cw, ch = fig.size
    sc = min(mw / cw, mh / ch)
    nw, nh = max(1, int(cw * sc)), max(1, int(ch * sc))
    fig = fig.resize((nw, nh), Image.LANCZOS)
    canvas = Image.new('RGBA', (W, H), (255, 255, 255, 0))
    canvas.alpha_composite(fig, ((W - nw) // 2, (H - nh) // 2))
    arr = np.array(canvas).astype('uint8')
    arr[..., 0] = arr[..., 1] = arr[..., 2] = 255   # whiten: shape stays in alpha
    return Image.fromarray(arr, 'RGBA')


def pad_palette(data, ncol=NCOL):
    """Pad the encoded .img.xbx palette up to `ncol` entries so it byte-matches the
    stock 2144-byte / 16-colour layout. Unused entries appended at the end never change
    the pixel indices, and the loader reads a fixed 16-colour palette for these sprites."""
    palsize = struct.unpack_from('<I', data, 28)[0]
    have = palsize // 4
    if have >= ncol:
        return data
    pal_end = 32 + palsize
    pal = data[32:pal_end] + b'\x00' * ((ncol - have) * 4)
    hdr = bytearray(data[:32])
    struct.pack_into('<I', hdr, 28, ncol * 4)     # new palsize
    return bytes(hdr) + pal + data[pal_end:]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--ska', default=DEF_SKA)
    ap.add_argument('--gamedir', default=DEF_GAME)
    ap.add_argument('--pose', default='relaxed')
    ap.add_argument('--skip-render', action='store_true',
                    help='reuse a previously rendered _render_raw.png in the work dir')
    ap.add_argument('--out', default=DEF_OUT)
    ap.add_argument('--workdir', default='/tmp/_vvport')
    a = ap.parse_args()

    raw = os.path.join(a.workdir, '_render_raw.png')
    if not a.skip_render:
        if not os.path.exists(a.ska):
            raise SystemExit('save not found: %s (pass --ska)' % a.ska)
        raw = render_raw(a.ska, a.gamedir, a.pose, a.workdir)
    if not os.path.exists(raw):
        raise SystemExit('no render at %s (drop --skip-render)' % raw)

    sil = silhouette(raw)
    sil_png = os.path.join(a.workdir, 'ss_vv_src.png')
    sil.save(sil_png)
    data = pad_palette(png2img.encode(sil_png, size=(W, H), colors=NCOL))

    os.makedirs(os.path.dirname(a.out), exist_ok=True)
    open(a.out, 'wb').write(data)
    print('wrote %s (%d bytes, %dx%d white silhouette)' % (a.out, len(data), W, H))


if __name__ == '__main__':
    main()
