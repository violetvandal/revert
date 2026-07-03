#!/usr/bin/env python3
# Recolor ONE texture inside a THUG2 .tex.xbx in place, byte-layout-preserving.
# Works on DXT1 color blocks: transforms the two RGB565 endpoint colors of every
# block (all mip levels) and leaves the 2-bit pixel indices untouched, so the
# texture's shading/detail (folds, shadows) is preserved and only the hue changes.
# Block MODE (4-color vs 3-color+punchthrough) is preserved by re-ordering
# endpoints + remapping indices when a transform would flip c0/c1 ordering.
#
# Usage:
#   tex_recolor.py <in.tex.xbx> <checksum-hex> <out.tex.xbx> tint  R G B
#   tex_recolor.py <in.tex.xbx> <checksum-hex> <out.tex.xbx> hsv   H S V   (H 0-360, S 0-100, V scale 0-200)
import struct, sys, colorsys

def c565_to_rgb(c):
    r=(c>>11)&0x1f; g=(c>>5)&0x3f; b=c&0x1f
    return ((r<<3)|(r>>2),(g<<2)|(g>>4),(b<<3)|(b>>2))

def rgb_to_c565(r,g,b):
    r=max(0,min(255,int(round(r)))); g=max(0,min(255,int(round(g)))); b=max(0,min(255,int(round(b))))
    return ((r>>3)<<11)|((g>>2)<<5)|(b>>3)

def make_xform(mode, a,b,c):
    if mode=='tint':
        # multiply tint: white fabric -> target color, shadows -> darker shade of target
        tr,tg,tb=a/255.0,b/255.0,c/255.0
        return lambda r,g,bl:(r*tr, g*tg, bl*tb)
    elif mode=='hsv':
        # replace hue+sat, keep relative brightness (value scaled by c/100)
        H,S,Vscale=a/360.0,b/100.0,c/100.0
        def f(r,g,bl):
            _,_,v=colorsys.rgb_to_hsv(r/255.0,g/255.0,bl/255.0)
            nr,ng,nb=colorsys.hsv_to_rgb(H,S,min(1.0,v*Vscale))
            return (nr*255,ng*255,nb*255)
        return f
    raise SystemExit("mode must be tint|hsv")

# remap a 32-bit index word (16x 2-bit) when endpoints are swapped
def remap_bits(bits, mapping):
    out=0
    for i in range(16):
        idx=(bits>>(2*i))&3
        out |= (mapping[idx] << (2*i))
    return out

MAP4 = [1,0,3,2]   # 4-color: swap c0<->c1
MAP3 = [1,0,2,3]   # 3-color: swap c0<->c1 (midpoint+transparent fixed)

def recolor_dxt1_color_block(buf, off, xform):
    c0,c1 = struct.unpack_from('<HH', buf, off)
    bits  = struct.unpack_from('<I', buf, off+4)[0]
    mode4 = c0 > c1
    nc0 = rgb_to_c565(*xform(*c565_to_rgb(c0)))
    nc1 = rgb_to_c565(*xform(*c565_to_rgb(c1)))
    if mode4:
        if nc0 == nc1:
            nc1 = nc1-1 if nc1>0 else 0
            if nc0 <= nc1: nc0 = nc1+1
        if nc0 < nc1:
            nc0,nc1 = nc1,nc0; bits = remap_bits(bits, MAP4)
    else:
        if nc0 > nc1:
            nc0,nc1 = nc1,nc0; bits = remap_bits(bits, MAP3)
    struct.pack_into('<HH', buf, off, nc0, nc1)
    struct.pack_into('<I', buf, off+4, bits)

def main():
    inp, ckhex, outp, mode = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
    a,b,c = (float(x) for x in sys.argv[5:8])
    target = int(ckhex,16) & 0xffffffff
    xform = make_xform(mode,a,b,c)
    d = bytearray(open(inp,'rb').read())
    ver,n = struct.unpack_from('<ii', d, 0); o=8
    hit=False
    for i in range(n):
        ck,w,h,lv,td,pd,dxt,ps = struct.unpack_from('<8I', d, o); o+=32
        if ps>0: o+=ps
        do_it = (ck==target)
        for l in range(lv):
            ds = struct.unpack_from('<I', d, o)[0]; o+=4
            start=o
            if do_it:
                if dxt!=1: raise SystemExit("texture %08x is dxt=%d, only dxt1 supported"%(ck,dxt))
                # DXT1: 8-byte blocks, color-only
                for bo in range(start, start+ds, 8):
                    recolor_dxt1_color_block(d, bo, xform)
                hit=True
            o+=ds
    if not hit: raise SystemExit("texture %08x not found in %s"%(target,inp))
    open(outp,'wb').write(d)
    print("recolored %08x in %s -> %s (%d bytes)"%(target,inp,outp,len(d)))

if __name__=='__main__':
    main()
