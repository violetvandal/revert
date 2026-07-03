#!/usr/bin/env python3
# M5: skeletal pose the created skater (linear-blend skinning) out of T-pose.
import struct,os,sys,glob
import numpy as np
sys.path.insert(0,os.path.dirname(os.path.abspath(__file__)))
from tex2png import decode_tex
from img2png import decode_img
from skeleton import parse_ske, local_mats, quat_to_mat, T
from PIL import Image
import colorsys

ARM_DEG=float(os.environ.get('ARM_DEG','72'))   # how far to lower arms

# ---- empirical arm rig: classify arm bones by which verts they influence ----
import numpy as np
TC,VCOL,VN,VW,BILL=0x01,0x02,0x04,0x10,0x00800000
M_UVW,M_VCW,M_TXANIM=(1<<0),(1<<1),(1<<11)
class R:
    def __init__(s,b): s.b=b; s.o=0
    def rd(s,f): v=struct.unpack_from(f,s.b,s.o); s.o+=struct.calcsize(f); return v
    def u8(s):return s.rd("B")[0]
    def u16(s):return s.rd("H")[0]
    def u32(s):return s.rd("I")[0]
    def i32(s):return s.rd("i")[0]
    def f32(s):return s.rd("f")[0]
    def b1(s):return s.rd("?")[0]
def _readmats(r,n):
    m2t={}
    for _ in range(n):
        mc=r.u32();r.u32();ps=r.u32();r.u32();r.b1();r.f32();r.b1();r.b1();r.i32()
        if r.b1(): r.f32();r.i32()
        if r.f32()>0.0: r.rd("3f")
        ft=None
        for j in range(ps):
            tex=r.u32();fl=r.u32();r.b1();r.rd("3f");r.u32();r.u32();r.u32();r.u32();r.rd("2f");r.u32()
            if fl&M_UVW: r.rd("8f")
            if j==0 and fl&M_VCW:
                for _ in range(r.u32()): nk=r.u32();r.i32();r.rd("%di"%(nk*2))
            if fl&M_TXANIM:
                nk=r.i32();r.i32();r.i32();r.i32()
                for _ in range(nk): r.u32();r.u32()
            if tex: r.u32();r.u32();r.f32();r.f32()
            else: r.rd("4I")
            if j==0: ft=tex
        m2t[mc]=ft
    return m2t
def parse_skin(path):
    r=R(open(path,"rb").read()); r.rd("3I"); m2t=_readmats(r,r.u32()); nsec=r.i32(); meshes=[]
    for _ in range(nsec):
        r.u32(); r.i32(); flags=r.u32(); nmesh=r.u32(); r.rd("3f"); r.rd("3f"); r.rd("4f")
        if flags&BILL: r.u32(); r.rd("3f"); r.rd("3f"); r.rd("3f")
        for _ in range(nmesh):
            r.rd("3f"); r.f32(); r.rd("3f"); r.rd("3f"); r.u32(); matck=r.u32(); nlod=r.u32()
            for k in range(nlod):
                n1=r.u32(); r.rd("%dH"%n1); n2=r.u16(); idx=r.rd("%dH"%n2); r.rd("14x")
                stride=r.u8(); nv=r.u16(); nbuf=r.u16(); V=[];UV=[];W=[]
                for bi in range(nbuf):
                    if bi>0: r.o+=1
                    r.i32()
                    for vi in range(nv):
                        pos=r.rd("3f"); used=12; wts=None
                        if flags&VW:
                            pw=r.u32(); bones=r.rd("4H"); used+=12
                            w=[(pw&0x7FF)/1023.0,((pw>>11)&0x7FF)/1023.0,((pw>>22)&0x3FF)/511.0,0.0]
                            wts=list(zip(w,[b//3 for b in bones]))
                            if flags&VN: r.u32(); used+=4
                        elif flags&VN: r.rd("3f"); used+=12
                        if flags&VCOL: r.rd("4B"); used+=4
                        uv=(0.0,0.0)
                        if flags&TC:
                            for mm in range((stride-used)//8):
                                tc=r.rd("2f")
                                if mm==0: uv=tc
                        if bi==0: V.append(pos);UV.append(uv);W.append(wts)
                F=[]
                for l in range(2,len(idx)):
                    a,b,c=(idx[l-2],idx[l],idx[l-1]) if l%2 else (idx[l-2],idx[l-1],idx[l])
                    if len({a,b,c})==3: F.append((a,b,c))
                meshes.append({'V':V,'UV':UV,'W':W,'F':F,'tex':m2t.get(matck)})
                r.i32(); r.i32(); r.rd("3B")
                if r.rd("B")[0]: r.o+=nv
                r.i32(); ps=r.i32()
                if ps==1: r.i32(); r.rd("%dB"%r.i32())
    return meshes

# stats pass over the body to classify bones (skin raw: x=lateral, y=up, z=fwd)
import json
SPEC=json.load(open(os.environ.get('SPEC','/tmp/skater_spec.json')))
GAMEDIR=os.environ.get('GAMEDIR','game-pristine-us')
def gpath(rel): return os.path.join(GAMEDIR, rel)
bsum={}; bcnt={}
for part in SPEC['parts']:
    sp=gpath(part['mesh'])
    if not os.path.exists(sp): continue
    for m in parse_skin(sp):
        for j,pos in enumerate(m['V']):
            for w,bi in (m['W'][j] or []):
                if w>0.4:
                    bsum[bi]=bsum.get(bi,np.zeros(3))+np.array(pos); bcnt[bi]=bcnt.get(bi,0)+1
bmean={bi:bsum[bi]/bcnt[bi] for bi in bsum}
# arm bones: upper body (y>48) AND lateral (|x|>6); split upper-arm vs forearm by distance
LEFT=set(bi for bi,mp in bmean.items() if mp[1]>48 and mp[0]<-6)
RIGHT=set(bi for bi,mp in bmean.items() if mp[1]>48 and mp[0]>6)
HEAD=set(bi for bi,mp in bmean.items() if abs(mp[0])<6 and mp[1]>63)
def split(side):
    if not side: return set(),set(),None,None
    xs=sorted(abs(bmean[b][0]) for b in side); thr=14.0
    upper=set(b for b in side if abs(bmean[b][0])<=thr)
    fore =set(b for b in side if abs(bmean[b][0])> thr)
    sh=min(side,key=lambda b:abs(bmean[b][0]))           # shoulder = innermost
    el=min(fore,key=lambda b:abs(bmean[b][0])) if fore else sh  # elbow = innermost forearm
    return upper,fore,bmean[sh].copy(),bmean[el].copy()
Lup,Lfore,LshP,LelP=split(LEFT); Rup,Rfore,RshP,RelP=split(RIGHT)
headP=min(HEAD,key=lambda b:bmean[b][1]) if HEAD else None
headP=bmean[headP].copy() if headP is not None else np.array([0,63,0])
def rot_about(point, axis, deg):
    a=np.array(axis,float); a/=np.linalg.norm(a); th=np.radians(deg)
    c,s=np.cos(th),np.sin(th); x,y,z=a
    Rm=np.array([[c+x*x*(1-c),x*y*(1-c)-z*s,x*z*(1-c)+y*s,0],
                 [y*x*(1-c)+z*s,c+y*y*(1-c),y*z*(1-c)-x*s,0],
                 [z*x*(1-c)-y*s,z*y*(1-c)+x*s,c+z*z*(1-c),0],[0,0,0,1]])
    T=np.eye(4); T[:3,3]=point; Ti=np.eye(4); Ti[:3,3]=-np.array(point)
    return T@Rm@Ti

# ---- pose spec (env POSE selects; angles in degrees) ----
import json
POSES={
 'relaxed': dict(Ls=68,Rs=68),
 'casual':  dict(Ls=58,Rs=76,head=8),
 'open':    dict(Ls=82,Rs=82,head=-4),
 'crossed': dict(Ls=30,Rs=30,Le=95,Re=95,head=5),
}
P=POSES.get(os.environ.get('POSE','relaxed'),POSES['relaxed'])
Ls=P.get('Ls',68); Rs=P.get('Rs',68); Le=P.get('Le',0); Re=P.get('Re',0); HEADT=P.get('head',0)
# shoulder rotations (about skin z; LEFT +, RIGHT -) ; elbow about lateral x (forward swing)
S_L=rot_about(LshP,(0,0,1),+Ls); S_R=rot_about(RshP,(0,0,1),-Rs)
LelP_p=(S_L@np.append(LelP,1))[:3]; RelP_p=(S_R@np.append(RelP,1))[:3]
F_L=rot_about(LelP_p,(0,0,1),-Le)@S_L; F_R=rot_about(RelP_p,(0,0,1),+Re)@S_R
H_R=rot_about(headP,(0,0,1),HEADT)
def coord(v): return (v[0],-v[2],v[1])
def wsum(wts,bset):
    return sum(w for w,bi in (wts or []) if w>0 and bi in bset)
def skin_vertex(pos,wts):
    v=np.array([pos[0],pos[1],pos[2],1.0])
    lu=wsum(wts,Lup); lf=wsum(wts,Lfore); ru=wsum(wts,Rup); rf=wsum(wts,Rfore); hd=wsum(wts,HEAD)
    tot=lu+lf+ru+rf+hd
    if tot<=0.001: return coord(pos)
    out=(1.0-min(1.0,tot))*v
    if lu>0: out=out+lu*(S_L@v)
    if lf>0: out=out+lf*(F_L@v)
    if ru>0: out=out+ru*(S_R@v)
    if rf>0: out=out+rf*(F_R@v)
    if hd>0: out=out+hd*(H_R@v)
    return coord((out[0],out[1],out[2]))
print("POSE=%s  L=%s R=%s"%(os.environ.get('POSE','relaxed'),sorted(LEFT),sorted(RIGHT)))

# ---- textures (reuse build_skater logic) ----
def tint_rgba(rgba,hsv):
    H,Sx,V=hsv[0]/360.0,hsv[1]/100.0,hsv[2]/100.0; out=bytearray(rgba)
    for i in range(0,len(out),4):
        _,_,tv=colorsys.rgb_to_hsv(out[i]/255,out[i+1]/255,out[i+2]/255)
        nr,ng,nb=colorsys.hsv_to_rgb(H,Sx,min(1.0,tv*V*1.6))
        out[i]=int(nr*255);out[i+1]=int(ng*255);out[i+2]=int(nb*255)
    return bytes(out)

# ---- spec-driven build (any skater) ----
OUT='tools/save/renders'; os.makedirs(OUT+'/tex',exist_ok=True)
obj=open(OUT+'/skater_posed.obj','w'); mtl=open(OUT+'/skater_posed.mtl','w'); obj.write("mtllib skater_posed.mtl\n")
vbase=0; mats=set()
for part in SPEC['parts']:
    skinp=gpath(part['mesh'])
    if not os.path.exists(skinp): print("skip(missing)",part['mesh']); continue
    texp=skinp[:-9]+'.tex.xbx'
    texs=decode_tex(texp) if os.path.exists(texp) else {}
    tint=part.get('tint'); face_with=part.get('face_with')
    face_ck=None
    if face_with and texs:
        face_ck=max(texs, key=lambda c: texs[c][0]*texs[c][1])   # largest tex = the face base
    for ck,(w,h,rgba) in list(texs.items()):
        if face_ck is not None and ck==face_ck and os.path.exists(gpath(face_with)):
            w,h,rgba=decode_img(gpath(face_with))
        if tint: rgba=tint_rgba(rgba,tint)
        Image.frombytes('RGBA',(w,h),rgba).transpose(Image.FLIP_TOP_BOTTOM).save("%s/tex/%08x.png"%(OUT,ck))
    pref=os.path.splitext(os.path.basename(part['mesh']))[0]
    for mi,m in enumerate(parse_skin(skinp)):
        mn="%s_%d"%(pref,mi); obj.write("g %s\nusemtl %s\n"%(mn,mn))
        if mn not in mats:
            mats.add(mn); mtl.write("newmtl %s\n"%mn)
            if m['tex'] and m['tex'] in texs: mtl.write("map_Kd tex/%08x.png\nKd 1 1 1\n"%m['tex'])
            else: mtl.write("Kd 0.7 0.7 0.7\n")
        for j,pos in enumerate(m['V']):
            x,y,z=skin_vertex(pos,m['W'][j]); obj.write("v %f %f %f\n"%(x,y,z))
        for u,v in m['UV']: obj.write("vt %f %f\n"%(u,v))
        for a,b,c in m['F']:
            obj.write("f %d/%d %d/%d %d/%d\n"%(vbase+a+1,vbase+a+1,vbase+b+1,vbase+b+1,vbase+c+1,vbase+c+1))
        vbase+=len(m['V'])
obj.close(); mtl.close()
print("posed OBJ written (spec=%s, POSE=%s, %d parts, %d materials)"%(SPEC.get('save','?'),os.environ.get('POSE','relaxed'),len(SPEC['parts']),len(mats)))
