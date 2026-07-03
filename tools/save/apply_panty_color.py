#!/usr/bin/env python3
# Recolor the female created-skater's PANTIES (THUG2: Violet Vandal Edition).
#
# WHY THIS IS WEIRD (hard-won, 2026-06-20): the visible panty is NOT the obvious
# panty texture (65b3576c) — that mesh is hidden behind the legs. When a female
# wears a skirt with `shows_panties`/`force_lower_legs_full`, the engine swaps the
# lower legs to `Skater_F_PVLegs` ("PV" = panty-visible). The panty you see is the
# WHITE REGION of that mesh's bundled leg-skin texture `bb850270` inside
# Skater_F_pvlegs.tex.xbx. So we recolor only the near-white DXT1 blocks of
# bb850270 -> the chosen colour, leaving the tan thigh skin untouched.
#
# Usage: apply_panty_color.py <gamedir> [R G B]   (default R G B = 130 50 190 purple)
import sys, os, struct
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'prx'))
sys.path.insert(0, os.path.dirname(__file__))
import prx, lzss
import tex_recolor as T

PVLEGS_ENTRY = 'models\\skater_female\\Skater_F_pvlegs.tex.xbx'
LEG_TEX_CK   = 0xbb850270   # the leg-skin texture; its white region is the panty
WHITE_THRESH = 170          # blocks with both endpoints' min-channel >= this -> recolour
ARCHIVES     = ['skaterparts_temp.prx', 'AuTempProfile.prx']  # both hold pvlegs

def recolor_white_blocks(raw, rgb):
    d = bytearray(raw); ver, n = struct.unpack_from('<ii', d, 0); o = 8; hit = 0
    xform = lambda r, g, b: (rgb[0], rgb[1], rgb[2])
    for _ in range(n):
        ck, w, h, lv, td, pd, dxt, ps = struct.unpack_from('<8I', d, o); o += 32
        if ps > 0: o += ps
        do = (ck == LEG_TEX_CK)
        for l in range(lv):
            ds = struct.unpack_from('<I', d, o)[0]; o += 4; start = o
            if do and dxt == 1:
                for bo in range(start, start + ds, 8):
                    c0, c1 = struct.unpack_from('<HH', d, bo)
                    r0, g0, b0 = T.c565_to_rgb(c0); r1, g1, b1 = T.c565_to_rgb(c1)
                    if min(r0, g0, b0) >= WHITE_THRESH and min(r1, g1, b1) >= WHITE_THRESH:
                        T.recolor_dxt1_color_block(d, bo, xform); hit += 1
            o += ds
    return bytes(d), hit

def apply(gamedir, rgb):
    for arc in ARCHIVES:
        path = os.path.join(gamedir, 'Data', 'pre', arc)
        if not os.path.exists(path):
            print('  skip (missing): %s' % arc); continue
        ver, entries = prx.parse(open(path, 'rb').read())
        e = prx.find(entries, PVLEGS_ENTRY)
        if not e:
            print('  skip (no pvlegs): %s' % arc); continue
        raw = lzss.decompress(e['blob'][:e['csize']], e['dsize']) if e['csize'] else e['blob'][:e['dsize']]
        new, hit = recolor_white_blocks(raw, rgb)
        comp = lzss.compress(new); assert lzss.decompress(comp, len(new)) == new, 'lzss roundtrip'
        e['dsize'] = len(new); e['csize'] = len(comp)
        e['blob'] = comp + b'\0' * (((len(comp) + 3) & ~3) - len(comp))
        open(path, 'wb').write(prx.build(ver, entries))
        print('  %-22s panty: %d white blocks -> rgb%s' % (arc, hit, tuple(rgb)))

if __name__ == '__main__':
    if len(sys.argv) < 2:
        sys.exit('usage: apply_panty_color.py <gamedir> [R G B]')
    gamedir = sys.argv[1]
    rgb = [int(x) for x in sys.argv[2:5]] if len(sys.argv) >= 5 else [130, 50, 190]
    apply(gamedir, rgb)
