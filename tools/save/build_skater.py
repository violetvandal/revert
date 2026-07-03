#!/usr/bin/env python3
# M4: build a TEXTURED OBJ+MTL of the created skater from its part .skin.xbx + .tex.xbx.
# Reuses skin2obj byte layout (with per-mesh material capture + UVs) and tex2png DXT decode.
import struct,sys,os
sys.path.insert(0,os.path.dirname(os.path.abspath(__file__)))
from tex2png import decode_tex
from img2png import decode_img
from PIL import Image

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
TC,VCOL,VN,VW,BILL=0x01,0x02,0x04,0x10,0x00800000
M_UVW,M_VCW,M_TXANIM=(1<<0),(1<<1),(1<<11)

def read_materials(r,n):
    mat2tex={}
    for _ in range(n):
        matck=r.u32(); r.u32(); passes=r.u32()
        r.u32(); r.b1(); r.f32(); r.b1(); r.b1(); r.i32()
        if r.b1(): r.f32(); r.i32()
        if r.f32()>0.0: r.rd("3f")
        first_tex=None
        for j in range(passes):
            tex=r.u32(); flags=r.u32(); r.b1(); r.rd("3f"); r.u32(); r.u32(); r.u32(); r.u32(); r.rd("2f"); r.u32()
            if flags&M_UVW: r.rd("8f")
            if j==0 and flags&M_VCW:
                for _ in range(r.u32()): nk=r.u32(); r.i32(); r.rd("%di"%(nk*2))
            if flags&M_TXANIM:
                nk=r.i32(); r.i32(); r.i32(); r.i32()
                for _ in range(nk): r.u32(); r.u32()
            if tex: r.u32(); r.u32(); r.f32(); r.f32()
            else: r.rd("4I")
            if j==0: first_tex=tex
        mat2tex[matck]=first_tex
    return mat2tex

def parse_skin(path):
    r=R(open(path,"rb").read()); r.rd("3I")
    mat2tex=read_materials(r,r.u32())
    nsec=r.i32(); meshes=[]
    for _ in range(nsec):
        r.u32(); r.i32(); flags=r.u32(); nmesh=r.u32(); r.rd("3f"); r.rd("3f"); r.rd("4f")
        if flags&BILL: r.u32(); r.rd("3f"); r.rd("3f"); r.rd("3f")
        for _ in range(nmesh):
            r.rd("3f"); r.f32(); r.rd("3f"); r.rd("3f"); r.u32(); matck=r.u32(); nlod=r.u32()
            for k in range(nlod):
                n1=r.u32(); r.rd("%dH"%n1); n2=r.u16(); idx=r.rd("%dH"%n2); r.rd("14x")
                stride=r.u8(); nv=r.u16(); nb=r.u16()
                V=[]; UV=[]
                for bi in range(nb):
                    if bi>0: r.o+=1
                    r.i32()
                    for vi in range(nv):
                        pos=coord(r.rd("3f")); used=12
                        if flags&VW:
                            r.u32(); r.rd("4H"); used+=12
                            if flags&VN: r.u32(); used+=4
                        elif flags&VN: r.rd("3f"); used+=12
                        if flags&VCOL: r.rd("4B"); used+=4
                        uv=(0.0,0.0)
                        if flags&TC:
                            for m in range((stride-used)//8):
                                tc=r.rd("2f")
                                if m==0: uv=tc
                        if bi==0: V.append(pos); UV.append(uv)
                F=[]
                for l in range(2,len(idx)):
                    a,b,c=(idx[l-2],idx[l],idx[l-1]) if l%2 else (idx[l-2],idx[l-1],idx[l])
                    if len({a,b,c})==3: F.append((a,b,c))
                meshes.append({'V':V,'UV':UV,'F':F,'tex':mat2tex.get(matck)})
                r.i32(); r.i32(); r.rd("3B")
                if r.rd("B")[0]: r.o+=nv
                r.i32(); ps=r.i32()
                if ps==1: r.i32(); r.rd("%dB"%r.i32())
    return meshes


import colorsys
# per-part HSV tint (h 0-360, s/v 0-100), keyed by skin-file basename (lowercase).
# Only colored clothing/hair/accessories; skin/face/body left untinted.
TINT={
 'hair_f_short':(265,90,60),'shirt_button_open_ss':(0,0,60),'pant_miniskirt':(155,0,12),
 'shoe_wrestler':(0,0,12),'extra_spikeband2_l':(0,0,12),'extra_wings':(0,0,12),
}
def tint_rgba(rgba,w,h_,hsv):
    H,S,V=hsv[0]/360.0,hsv[1]/100.0,hsv[2]/100.0
    out=bytearray(rgba)
    for i in range(0,len(out),4):
        r,g,b=out[i]/255,out[i+1]/255,out[i+2]/255
        _,_,tv=colorsys.rgb_to_hsv(r,g,b)
        nv=min(1.0, tv*V*1.6)              # scale texture brightness by slot value
        nr,ng,nb=colorsys.hsv_to_rgb(H,S,nv)
        out[i]=int(nr*255);out[i+1]=int(ng*255);out[i+2]=int(nb*255)
    return bytes(out)


# Face-texture override: the selected head variant ("Goth") swaps the face texture
# (mesh uses 5fd7a594) for its CAS "with=" image (CS_MBF_F_Gry_HEAD17 = goth makeup/face paint).
OVERRIDE_TEX={0x5fd7a594:'game-pristine-us/Data/textures/Skater_male/CS_MBF_F_Gry_HEAD17.img.xbx'}

# ---- build combined textured OBJ ----
GD='game-pristine-us/Data/models/'
PARTS=['Skater_female/skater_female','Skater_female/Skater_F_Legs','Skater_female/head_Female_01',
 'Skater_female/Hair_F_Short','Skater_female/shirt_button_open_ss','Skater_female/pant_miniskirt',
 'Skater_female/Skater_F_Hands','Skater_Male/shoe_wrestler','Skater_Male/extra_Spikeband2_L',
 'Skater_Male/Extra_Watch_R_01','Skater_Male/extra_wings']
OUT='tools/save/renders'; os.makedirs(OUT+'/tex',exist_ok=True)
obj=open(OUT+'/skater_tex.obj','w'); mtl=open(OUT+'/skater_tex.mtl','w')
obj.write("mtllib skater_tex.mtl\n")
vbase=0; mats=set()
import glob
for p in PARTS:
    skin=glob.glob(GD+p+'.skin.xbx',recursive=False)
    skin=[x for x in [GD+p+'.skin.xbx'] if os.path.exists(x)]
    if not skin:
        # case-insensitive
        d=os.path.dirname(GD+p); bn=os.path.basename(p).lower()+'.skin.xbx'
        skin=[os.path.join(d,x) for x in os.listdir(d) if x.lower()==bn]
    if not skin: print("skip",p); continue
    skinp=skin[0]
    texp=skinp[:-len('.skin.xbx')]+'.tex.xbx'
    texs=decode_tex(texp) if os.path.exists(texp) else {}
    # save textures as png
    tint=TINT.get(os.path.basename(p).lower())
    for ck,(w,h,rgba) in texs.items():
        if ck in OVERRIDE_TEX:
            w,h,rgba=decode_img(OVERRIDE_TEX[ck])
        if tint: rgba=tint_rgba(rgba,w,h,tint)
        Image.frombytes('RGBA',(w,h),rgba).transpose(Image.FLIP_TOP_BOTTOM).save("%s/tex/%08x.png"%(OUT,ck))
    pref=os.path.basename(p)
    for mi,m in enumerate(parse_skin(skinp)):
        mname="%s_%d"%(pref,mi)
        tex=m['tex']
        obj.write("g %s\nusemtl %s\n"%(mname,mname))
        if mname not in mats:
            mats.add(mname); mtl.write("newmtl %s\n"%mname)
            if tex and tex in texs: mtl.write("map_Kd tex/%08x.png\nKd 1 1 1\n"%tex)
            else: mtl.write("Kd 0.7 0.7 0.7\n")
        for x,y,z in m['V']: obj.write("v %f %f %f\n"%(x,y,z))
        for u,v in m['UV']: obj.write("vt %f %f\n"%(u,v))
        for a,b,c in m['F']:
            obj.write("f %d/%d %d/%d %d/%d\n"%(vbase+a+1,vbase+a+1,vbase+b+1,vbase+b+1,vbase+c+1,vbase+c+1))
        vbase+=len(m['V'])
obj.close(); mtl.close()
print("wrote %s/skater_tex.obj (%d materials)"%(OUT,len(mats)))
