#!/usr/bin/env python3
# THUG2 .tex.xbx -> PNG(s). Decodes DXT1/DXT5 (S3TC), linear block order.
# Usage: tex2png.py <file.tex.xbx> <outdir>   -> writes <checksum>.png per texture
import struct,sys,os
from PIL import Image

def c565(c):
    r=(c>>11)&0x1f; g=(c>>5)&0x3f; b=c&0x1f
    return (r<<3)|(r>>2),(g<<2)|(g>>4),(b<<3)|(b>>2)

def dxt_block_colors(c0,c1,dxt1):
    r0,g0,b0=c565(c0); r1,g1,b1=c565(c1)
    cols=[(r0,g0,b0,255),(r1,g1,b1,255)]
    if c0>c1 or not dxt1:
        cols.append(((2*r0+r1)//3,(2*g0+g1)//3,(2*b0+b1)//3,255))
        cols.append(((r0+2*r1)//3,(g0+2*g1)//3,(b0+2*b1)//3,255))
    else:
        cols.append(((r0+r1)//2,(g0+g1)//2,(b0+b1)//2,255))
        cols.append((0,0,0,0))
    return cols

def decode_dxt(data,w,h,dxt5):
    out=bytearray(w*h*4)
    o=0; bw=(w+3)//4; bh=(h+3)//4
    for by in range(bh):
        for bx in range(bw):
            if dxt5:
                a0,a1=data[o],data[o+1]
                abits=int.from_bytes(data[o+2:o+8],'little')
                alpha=[a0,a1]
                if a0>a1:
                    for i in range(2,8): alpha.append(((8-i)*a0+(i-1)*a1)//7)
                else:
                    for i in range(2,6): alpha.append(((6-i)*a0+(i-1)*a1)//5)
                    alpha+=[0,255]
                o+=8
            c0,c1=struct.unpack_from('<HH',data,o); bits=struct.unpack_from('<I',data,o+4)[0]; o+=8
            cols=dxt_block_colors(c0,c1,not dxt5)
            for py in range(4):
                for px in range(4):
                    x=bx*4+px; y=by*4+py
                    if x>=w or y>=h: continue
                    idx=(bits>>(2*(py*4+px)))&3
                    r,g,b,a=cols[idx]
                    if dxt5:
                        ai=(abits>>(3*(py*4+px)))&7; a=alpha[ai]
                    p=(y*w+x)*4
                    out[p],out[p+1],out[p+2],out[p+3]=r,g,b,a
    return bytes(out)

def decode_tex(path):
    d=open(path,'rb').read(); ver,n=struct.unpack_from('<ii',d,0); o=8; res={}
    for i in range(n):
        ck,w,h,lv,td,pd,dxt,ps=struct.unpack_from('<8I',d,o); o+=32
        if ps>0: o+=ps
        first=None
        for l in range(lv):
            ds=struct.unpack_from('<I',d,o)[0]; o+=4
            if l==0: first=(d[o:o+ds],dxt); o+=ds
            else: o+=ds
        data,dxt=first
        d5 = (dxt==5)
        rgba=decode_dxt(data,w,h,d5)
        res[ck]=(w,h,rgba)
    return res

if __name__=='__main__':
    outdir=sys.argv[2]; os.makedirs(outdir,exist_ok=True)
    for ck,(w,h,rgba) in decode_tex(sys.argv[1]).items():
        im=Image.frombytes('RGBA',(w,h),rgba)
        im.save(os.path.join(outdir,"%08x.png"%ck))
        print("  %08x %dx%d"%(ck,w,h))
