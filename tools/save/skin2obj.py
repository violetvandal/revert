#!/usr/bin/env python3
# THUG2 .skin.xbx / .scn.xbx / .mdl.xbx  ->  Wavefront OBJ
# Byte layout ported from denetii/io_thps_scene (import_thug2.py + material.py).
import struct,sys,os
class R:
    def __init__(s,b): s.b=b; s.o=0
    def rd(s,f): v=struct.unpack_from(f,s.b,s.o); s.o+=struct.calcsize(f); return v
    def u8(s): return s.rd("B")[0]
    def u16(s): return s.rd("H")[0]
    def u32(s): return s.rd("I")[0]
    def i32(s): return s.rd("i")[0]
    def f32(s): return s.rd("f")[0]
    def b1(s): return s.rd("?")[0]
def coord(v): return (v[0],-v[2],v[1])
# flags
TC,VCOL,VCW,VN,VW,BILL = 0x01,0x02,0x800,0x04,0x10,0x00800000
M_UVW,M_VCW,M_TXANIM = (1<<0),(1<<1),(1<<11)

def skip_materials(r,n):
    for _ in range(n):
        r.u32(); r.u32()              # mat checksum, name checksum
        passes=r.u32()
        r.u32(); r.b1(); r.f32(); r.b1(); r.b1(); r.i32()   # alpha,sorted,draworder,single,noback,zbias
        if r.b1():                     # grassify
            r.f32(); r.i32()
        if r.f32()>0.0:                # specular power -> color
            r.rd("3f")
        for j in range(passes):
            tex=r.u32(); flags=r.u32(); r.b1(); r.rd("3f")   # texck,flags,hascolor,color
            r.u32(); r.u32()           # blend, fixed alpha
            r.u32(); r.u32()           # u,v addressing
            r.rd("2f"); r.u32()        # envmap, filtering
            if flags & M_UVW: r.rd("8f")
            if j==0 and flags & M_VCW:
                for _ in range(r.u32()):
                    nk=r.u32(); r.i32(); r.rd("%di"%(nk*2))
            if flags & M_TXANIM:
                nk=r.i32(); r.i32(); r.i32(); r.i32()
                for _ in range(nk): r.u32(); r.u32()
            if tex: r.u32(); r.u32(); r.f32(); r.f32()
            else:   r.rd("4I")

def parse(path):
    r=R(open(path,"rb").read())
    r.rd("3I")
    nmat=r.u32(); skip_materials(r,nmat)
    nsec=r.i32()
    V=[]; UV=[]; F=[]
    for si in range(nsec):
        r.u32()                        # sector checksum
        bone=r.i32(); flags=r.u32(); nmesh=r.u32()
        r.rd("3f"); r.rd("3f"); r.rd("4f")          # bbox, sphere
        if flags & BILL:
            r.u32(); r.rd("3f"); r.rd("3f"); r.rd("3f")
        for mi in range(nmesh):
            r.rd("3f"); r.f32(); r.rd("3f"); r.rd("3f")  # center,radius,bbox
            r.u32(); r.u32()           # mesh flags, mat checksum
            nlod=r.u32()
            for k in range(nlod):
                n1=r.u32(); r.rd("%dH"%n1)
                n2=r.u16(); idx=r.rd("%dH"%n2)
                r.rd("14x")
                stride=r.u8(); nverts=r.u16(); nbufs=r.u16()
                base=len(V)
                mesh_uv={}
                for bidx in range(nbufs):
                    if bidx>0: r.o+=1
                    r.i32()            # buf size
                    for vi in range(nverts):
                        pos=coord(r.rd("3f")); used=12
                        if flags & VW:
                            r.u32(); r.rd("4H"); used+=12
                            if flags & VN: r.u32(); used+=4
                        elif flags & VN:
                            r.rd("3f"); used+=12
                        if flags & VCOL: r.rd("4B"); used+=4
                        uv=None
                        if flags & TC:
                            for m in range((stride-used)//8):
                                tc=r.rd("2f")
                                if m==0: uv=tc
                        if bidx==0:
                            V.append(pos); UV.append(uv if uv else (0.0,0.0))
                # faces: triangle strip (indices into this mesh's verts, 0-based -> global base)
                for l in range(2,len(idx)):
                    a,b,c=idx[l-2],idx[l-1],idx[l]
                    if l%2: a,b,c=idx[l-2],idx[l],idx[l-1]
                    if len({a,b,c})<3: continue
                    F.append((base+a+1,base+b+1,base+c+1))   # OBJ 1-based
                # mesh trailer
                r.i32(); r.i32(); r.rd("3B")
                if r.rd("B")[0]: r.o+=nverts
                r.i32()                # num index sets
                ps=r.i32()
                if ps==1: r.i32(); r.rd("%dB"%r.i32())
    return V,UV,F

V,UV,F=parse(sys.argv[1])
out=sys.argv[2] if len(sys.argv)>2 else os.path.splitext(sys.argv[1])[0]+".obj"
with open(out,"w") as f:
    for x,y,z in V: f.write("v %f %f %f\n"%(x,y,z))
    for u,v in UV: f.write("vt %f %f\n"%(u,v))
    for a,b,c in F: f.write("f %d/%d %d/%d %d/%d\n"%(a,a,b,b,c,c))
print("OK: %d verts, %d faces -> %s"%(len(V),len(F),out))
