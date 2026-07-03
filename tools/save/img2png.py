#!/usr/bin/env python3
# THUG2 .img.xbx -> PNG. Palettized(swizzled 8-bit) or raw BGRA32. Ported from io_thps_scene.
import struct,sys,os
from PIL import Image
def swizzle_axis(val,mask):
    bit=1;res=0
    while bit<=mask:
        if mask&bit: res|=val&bit
        else: val<<=1
        bit<<=1
    return res
def masks(w,h):
    x=y=0;bit=1;idx=1
    while bit<w or bit<h:
        if bit<w: x|=idx;idx<<=1
        if bit<h: y|=idx;idx<<=1
        bit<<=1
    return x,y
def unswizzle8(data,w,h):
    mx,my=masks(w,h); out=bytearray(len(data))
    for y in range(h):
        for x in range(w):
            a=y*w+x; b=swizzle_axis(x,mx)|swizzle_axis(y,my)
            out[a]=data[b]
    return out
def decode_img(path):
    d=open(path,'rb').read()
    w=struct.unpack_from('<H',d,24)[0]; h=struct.unpack_from('<H',d,26)[0]
    palsize=struct.unpack_from('<I',d,28)[0]; o=32
    if palsize>0:
        pal=[]
        for j in range(palsize//4):
            cb,cg,cr,ca=d[o:o+4]; o+=4; pal.append((cr,cg,cb,ca))
        idx=unswizzle8(d[o:o+w*h],w,h)
        out=bytearray(w*h*4)
        for i,pi in enumerate(idx):
            r,g,b,a=pal[pi]; out[i*4:i*4+4]=bytes((r,g,b,a))
        return w,h,bytes(out)
    else:
        raw=d[o:o+w*h*4]; out=bytearray(w*h*4)
        for i in range(w*h):
            cb,cg,cr,ca=raw[i*4:i*4+4]; out[i*4:i*4+4]=bytes((cr,cg,cb,ca))
        return w,h,bytes(out)
if __name__=='__main__':
    w,h,rgba=decode_img(sys.argv[1])
    Image.frombytes('RGBA',(w,h),rgba).save(sys.argv[2])
    print("%dx%d -> %s"%(w,h,sys.argv[2]))
