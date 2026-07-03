#!/usr/bin/env python3
# png2img: encode a PNG into a THUG2 Xbox .img.xbx (palettized 8-bit, swizzled),
# matching the CAGR sprite format so it can be injected into cagpieces.prx and
# used as a Create-A-Graphic clip-art piece / custom tag.
# Inverse of tools/save/img2png.py.  Header: f00=2 f04=8 wU32 hU32 f10=0x13 0 wU16 hU16 palsizeU32
import struct, sys
import numpy as np
from PIL import Image

def swizzle_axis(val, mask):
    bit=1; res=0
    while bit<=mask:
        if mask&bit: res|=val&bit
        else: val<<=1
        bit<<=1
    return res

def masks(w,h):
    x=y=0; bit=1; idx=1
    while bit<w or bit<h:
        if bit<w: x|=idx; idx<<=1
        if bit<h: y|=idx; idx<<=1
        bit<<=1
    return x,y

def swizzle8(linear, w, h):
    """linear[y*w+x] -> swizzled[ swizzle_axis(x,mx) | swizzle_axis(y,my) ]"""
    mx,my = masks(w,h)
    out = bytearray(len(linear))
    for y in range(h):
        sy = swizzle_axis(y,my)
        for x in range(w):
            out[swizzle_axis(x,mx)|sy] = linear[y*w+x]
    return bytes(out)

def encode(png_path, size=(64,64), colors=256, flip_y=True):
    """PNG -> .img.xbx bytes (palettized, swizzled). size must be powers of two."""
    w,h = size
    im = Image.open(png_path).convert('RGBA').resize((w,h), Image.LANCZOS)
    if flip_y:                      # CAGR .img.xbx are stored vertically flipped
        im = im.transpose(Image.FLIP_TOP_BOTTOM)
    # quantize RGBA -> <=colors palette (FASTOCTREE keeps alpha)
    q = im.quantize(colors=colors, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.NONE)
    idx = np.asarray(q, dtype=np.uint8)                 # h x w palette indices
    npal = max(int(idx.max())+1, 1)
    # Build the palette directly from the SOURCE pixels of each cluster — PIL's
    # getpalette() does not align with the indices for RGBA FASTOCTREE quant.
    src = np.asarray(im, dtype=np.float32)              # h x w x 4 (RGBA)
    pal = bytearray()
    for i in range(npal):
        m = (idx==i)
        if m.any():
            r,g,b,a = (int(round(c)) for c in src[m].mean(axis=0))
        else:
            r=g=b=a=0
        pal += bytes((b,g,r,a))                          # BGRA
    palsize = len(pal)
    swz = swizzle8(idx.reshape(-1).tobytes(), w, h)
    hdr = struct.pack('<IIIIII', 2, 8, w, h, 0x13, 0) + struct.pack('<HHI', w, h, palsize)
    return hdr + bytes(pal) + swz

if __name__ == '__main__':
    png = sys.argv[1]; out = sys.argv[2]
    sz = int(sys.argv[3]) if len(sys.argv)>3 else 64
    data = encode(png, size=(sz,sz))
    open(out,'wb').write(data)
    print("wrote %s (%d bytes, %dx%d)"%(out, len(data), sz, sz))
