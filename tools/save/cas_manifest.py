#!/usr/bin/env python3
# M1+M2: read a THUG2 .SKA save -> resolve the created skater's appearance to a
# render manifest (part slot -> desc_id name -> .skin.xbx mesh file on disk).
# Usage: cas_manifest.py "<save.SKA>" [<gamedir>]
import sys,subprocess,struct,zlib,re,os,glob
HERE=os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0,os.path.join(HERE,'..','prx'))
import prx,lzss
def ck(s): return (zlib.crc32(s.lower().encode('latin1'))^0xffffffff)&0xffffffff
SAVE=sys.argv[1]; GAME=sys.argv[2] if len(sys.argv)>2 else 'game-pristine-us'
GD=GAME+'/Data'

# 1) decompile CAS scripts -> build desc_id checksum -> (name, mesh) map
os.makedirs('/tmp/cas',exist_ok=True)
ver,e=prx.parse(open(GAME+'/Data/pre/qb_scripts.prx','rb').read())
desc={}  # checksum -> name ;  mesh by name
mesh_by_name={}
for inner in ['scripts\\game\\cas_skater_m.qb','scripts\\game\\cas_skater_f.qb',
              'scripts\\game\\cas_skater_shared.qb','scripts\\game\\cas_parts.qb',
              'scripts\\game\\cas_logos.qb','scripts\\game\\casutils.qb']:
    x=prx.find(e,inner)
    if not x: continue
    data=lzss.decompress(x['blob'][:x['csize']],x['dsize']) if x['csize'] else x['blob'][:x['dsize']]
    f='/tmp/cas/'+inner.split('\\')[-1]; open(f,'wb').write(data)
    if subprocess.run(['tools/neverscript/ns','-d',f,'-o',f+'.ns'],capture_output=True).returncode: continue
    t=open(f+'.ns','rb').read().decode('latin1')
    for m in re.finditer(r'desc_id=(`[^`]+`|[\w]+)', t):
        nm=m.group(1).strip('`'); desc[ck(nm)]=nm
        mm=re.search(r'mesh="([^"]+)"', t[m.end():m.end()+400])
        if mm: mesh_by_name[nm]=mm.group(1)
    for s in re.findall(r'`([^`]+)`',t)+re.findall(r'[A-Za-z_][A-Za-z0-9_ ]{1,}',t):
        desc.setdefault(ck(s.strip()), s.strip())

# 2) case-insensitive .skin.xbx index
skin={}
for dp,_,fs in os.walk(GD+'/models'):
    for fn in fs:
        if fn.lower().endswith('.skin.xbx'):
            skin[(os.path.relpath(dp,GD)+'/'+fn).lower().replace('\\','/')]=os.path.join(dp,fn)
def resolve_file(meshpath): return skin.get((meshpath+'.xbx').lower())

# 3) parse the save appearance: slot field (0a/0d after 00) then 8d1e NAME values
d=open(SAVE,'rb').read()
ap=d.find(struct.pack('<I',ck('appearance')))-1
i=ap+5; cur='?'; rows=[]
while i<ap+0x600 and i<len(d)-6:
    b=d[i]
    if b in (0x0a,0x0d) and d[i-1]==0x00:
        cur=desc.get(struct.unpack_from('<I',d,i+1)[0],'#%08x'%struct.unpack_from('<I',d,i+1)[0]); i+=5; continue
    if b==0x8d and d[i+1]==0x1e:
        h=struct.unpack_from('<I',d,i+2)[0]; nm=desc.get(h,'#%08x'%h)
        if nm not in ('None',): rows.append((cur,nm)); i+=6; continue
    i+=1

print("=== %s — CREATED SKATER RENDER MANIFEST ===\n"%os.path.basename(SAVE))
seen=set()
for slot,part in rows:
    if (slot,part) in seen: continue
    seen.add((slot,part))
    mp=mesh_by_name.get(part)
    fp=resolve_file(mp) if mp else None
    if fp: tag=fp
    elif mp: tag="(mesh listed, file?) "+mp
    else: tag="(texture/graphic, no mesh)"
    print("  %-16s %-22s %s"%(slot, part, tag))
