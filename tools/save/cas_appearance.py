#!/usr/bin/env python3
# M1: read a THUG2 .SKA save and report the created-skater appearance selections.
# Resolves CStruct NAME values (8d 1e <hash>) + struct/NAME fields against a
# dictionary built from the game's CAS scripts + part model filenames.
import sys,subprocess,struct,zlib,re,os
sys.path.insert(0,os.path.join(os.path.dirname(__file__),'..','prx'))
import prx,lzss
def ck(s): return (zlib.crc32(s.lower().encode('latin1'))^0xffffffff)&0xffffffff

GAME=sys.argv[2] if len(sys.argv)>2 else 'game-pristine-us'
def build_dict():
    names={}
    ver,e=prx.parse(open(GAME+'/Data/pre/qb_scripts.prx','rb').read())
    for inner in ['scripts\\game\\cas_skater_m.qb','scripts\\game\\cas_skater_shared.qb',
                  'scripts\\game\\cas_parts.qb','scripts\\cas_ped_m.qb','scripts\\cas_ped_f.qb',
                  'scripts\\game\\cas_skater_f.qb','scripts\\game\\cas_logos.qb','scripts\\game\\casutils.qb']:
        x=prx.find(e,inner)
        if not x: continue
        data=lzss.decompress(x['blob'][:x['csize']],x['dsize']) if x['csize'] else x['blob'][:x['dsize']]
        f='/tmp/cas/'+inner.split('\\')[-1]; os.makedirs('/tmp/cas',exist_ok=True); open(f,'wb').write(data)
        r=subprocess.run(['tools/neverscript/ns','-d',f,'-o',f+'.ns'],capture_output=True)
        if r.returncode==0:
            t=open(f+'.ns','rb').read().decode('latin1')
            for tok in set(re.findall(r'[A-Za-z_][A-Za-z0-9_]{2,}',t)): names.setdefault(ck(tok),tok)
            for nm,hx in re.findall(r'([A-Za-z_]\w+)\s+#([0-9a-fA-F]{8})',t): names[int(hx,16)]=nm
    for r in ['Skater_Male','Skater_female','Skater_Pro','Skater_Secret']:
        base=GAME+'/Data/models/'+r
        if os.path.isdir(base):
            for dp,_,fs in os.walk(base):
                for f in fs:
                    for suf in ('.skin.xbx','.tex.xbx','.cas.xbx'):
                        if f.endswith(suf): names.setdefault(ck(f[:-len(suf)]),f[:-len(suf)])
    return names

names=build_dict()
def nm(h): return names.get(h, "#%08x"%h)
d=open(sys.argv[1],'rb').read()
ap=d.find(struct.pack('<I',ck('appearance')))-1
print("=== %s ==="%sys.argv[1])
print("appearance @0x%x  (%d names indexed)\n"%(ap,len(names)))
i=ap+5  # skip 0a + appearance hash
cur=None; out=[]
end=ap+0x600
while i<end and i<len(d)-4:
    b=d[i]
    if b in (0x0a,0x0d) and d[i-1]==0x00:  # named field
        h=struct.unpack_from('<I',d,i+1)[0]; cur=nm(h)
        out.append(("FIELD",cur)); i+=5; continue
    if b==0x8d and d[i+1]==0x1e:  # anon NAME value
        h=struct.unpack_from('<I',d,i+2)[0]; out.append(("  val",nm(h))); i+=6; continue
    i+=1
for kind,v in out:
    if kind=="FIELD": print("%s:"%v)
    else:
        if v!="None": print("    = %s"%v)
