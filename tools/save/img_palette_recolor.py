#!/usr/bin/env python3
# Recolor palette entries of a THUG2 palettized .img.xbx IN PLACE (byte-safe):
# only the BGRA palette table is modified; the swizzled index data is untouched,
# so format/size are identical. Targets entries by brightness threshold.
# recolor_img_bytes(raw, predicate, rgb) -> new_bytes
import struct
def recolor_img_bytes(raw, pred, rgb):
    d=bytearray(raw)
    palsize=struct.unpack_from('<I',d,28)[0]
    if palsize==0: return bytes(d),0
    o=32; n=0
    for j in range(palsize//4):
        cb,cg,cr,ca=d[o],d[o+1],d[o+2],d[o+3]
        if pred(cr,cg,cb,ca):
            d[o]=rgb[2]; d[o+1]=rgb[1]; d[o+2]=rgb[0]   # BGRA, keep alpha
            n+=1
        o+=4
    return bytes(d),n
