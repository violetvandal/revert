#!/usr/bin/env python3
# caslib: generic THUG2 created-skater support.
#  - build_catalog(): decompile the game's CAS scripts -> {desc_hash: {name, mesh, with_tex, in}}
#    + a checksum->name dictionary. Cached to cas_catalog.json (regenerable).
#  - parse_save(): read any .SKA -> gender + ordered list of part selections (desc + HSV).
import os, sys, json, struct, zlib, re, subprocess, glob
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, '..', 'prx'))
import prx, lzss

def cksum(s):  # Neversoft StringToChecksum (CRC32, no final XOR), case-insensitive
    return (zlib.crc32(s.lower().encode('latin1')) ^ 0xffffffff) & 0xffffffff

CAS_QBS = ['scripts\\game\\cas_skater_m.qb','scripts\\game\\cas_skater_f.qb',
           'scripts\\game\\cas_skater_shared.qb','scripts\\game\\cas_parts.qb',
           'scripts\\cas_ped_m.qb','scripts\\cas_ped_f.qb',
           'scripts\\game\\cas_logos.qb','scripts\\game\\casutils.qb']

def build_catalog(gamedir, ns_bin=None, cache=None):
    cache = cache or os.path.join(HERE, 'cas_catalog.json')
    ns_bin = ns_bin or os.path.join(HERE, '..', 'neverscript', 'ns')
    names = {}     # hash -> readable name
    parts = {}     # desc_name -> {mesh, with_tex, replace_tex, in}
    ver, e = prx.parse(open(os.path.join(gamedir,'Data','pre','qb_scripts.prx'),'rb').read())
    tmp = '/tmp/caslib'; os.makedirs(tmp, exist_ok=True)
    for inner in CAS_QBS:
        x = prx.find(e, inner)
        if not x: continue
        data = lzss.decompress(x['blob'][:x['csize']], x['dsize']) if x['csize'] else x['blob'][:x['dsize']]
        qf = os.path.join(tmp, inner.split('\\')[-1]); open(qf,'wb').write(data)
        if subprocess.run([ns_bin,'-d',qf,'-o',qf+'.ns'], capture_output=True).returncode != 0: continue
        t = open(qf+'.ns','rb').read().decode('latin1')
        # name dictionary: identifiers, backtick + quoted strings
        for tok in set(re.findall(r'[A-Za-z_][A-Za-z0-9_ ]{1,}', t)): names.setdefault(cksum(tok.strip()), tok.strip())
        for s in re.findall(r'`([^`]+)`', t)+re.findall(r'"([^"]+)"', t)+re.findall(r'%"([^"]+)"', t):
            names.setdefault(cksum(s), s)
        # part definitions: desc_id=`X` ... (mesh="Y") (with="Z") (replace="W") (in=K)
        gtag = 'female' if 'cas_skater_f' in inner or 'ped_f' in inner else ('male' if 'cas_skater_m' in inner or 'ped_m' in inner else 'any')
        for m in re.finditer(r'desc_id=(`[^`]+`|[\w]+)', t):
            nm = m.group(1).strip('`'); tail = t[m.end():m.end()+400]
            d = parts.setdefault(nm, {'meshes':[]})
            mm = re.search(r'mesh="([^"]+)"', tail);  wm = re.search(r'with="([^"]+)"', tail)
            im = re.search(r'\bin=(\w+)', tail)
            if mm: d['meshes'].append([mm.group(1), gtag])
            if wm and 'with' not in d: d['with'] = wm.group(1)
            if im and 'in' not in d: d['in'] = im.group(1)
    # desc_hash -> info
    by_hash = {}
    for nm, info in parts.items():
        info = dict(info); info['name'] = nm; by_hash['%08x'%cksum(nm)] = info
    out = {'names': {('%08x'%k): v for k,v in names.items()}, 'parts': by_hash}
    json.dump(out, open(cache,'w'))
    return out

def load_catalog(gamedir=None, rebuild=False):
    cache = os.path.join(HERE, 'cas_catalog.json')
    if rebuild or not os.path.exists(cache):
        if not gamedir: raise RuntimeError("catalog missing; pass gamedir to build it")
        return build_catalog(gamedir)
    return json.load(open(cache))

# ---------- save parsing ----------
def parse_save(path, catalog):
    d = open(path,'rb').read()
    ap = d.find(struct.pack('<I', cksum('appearance')))
    if ap < 0: raise RuntimeError("no appearance block in save")
    ap -= 1
    names = catalog['names']
    sel = []  # ordered part selections
    i = ap; end = min(len(d)-8, ap + 0x800)
    while i < end:
        # slot record: 8a <slotid> 8d 1e <desc:4> [fields...] 00
        if d[i]==0x8a and d[i+2]==0x8d and d[i+3]==0x1e:
            desc = struct.unpack_from('<I', d, i+4)[0]
            hx = '%08x'%desc
            if hx in names or hx in catalog['parts']:
                rec = {'desc': hx, 'name': names.get(hx, '#'+hx)}
                j = i+8
                while j < end and d[j] != 0x00:
                    t = d[j]
                    if t in (0x90,0x91,0x92):
                        fid = d[j+1]
                        if t==0x92: val=0; j+=2
                        elif t==0x90: val=d[j+2]; j+=3
                        else: val=struct.unpack_from('<H',d,j+2)[0]; j+=4
                        rec[{0x1f:'h',0x20:'s',0x21:'v',0x22:'udh'}.get(fid, 'f%02x'%fid)] = val
                    else: j += 1
                sel.append(rec); i = j+1; continue
        i += 1
    # gender: FemaleBody vs MaleBody present in selections/save
    fem = struct.pack('<I',cksum('FemaleBody')) in d
    mal = struct.pack('<I',cksum('MaleBody')) in d
    gender = 'female' if fem and not mal else ('male' if mal and not fem else ('female' if fem else 'male'))
    return {'gender': gender, 'selections': sel}

if __name__ == '__main__':
    gamedir = sys.argv[2] if len(sys.argv) > 2 else 'game-pristine-us'
    cat = load_catalog(gamedir, rebuild=('--rebuild' in sys.argv))
    print("catalog: %d names, %d parts"%(len(cat['names']), len(cat['parts'])))
    if len(sys.argv) > 1 and sys.argv[1] not in ('--rebuild',):
        s = parse_save(sys.argv[1], cat)
        print("\n%s  (gender=%s)"%(os.path.basename(sys.argv[1]), s['gender']))
        for r in s['selections']:
            info = cat['parts'].get(r['desc'], {})
            hsv = (' h%s s%s v%s'%(r.get('h',0),r.get('s',0),r.get('v',0))) if 'h' in r or 'v' in r else ''
            mesh = info.get('mesh','')
            if r['name']!='None': print("  %-20s%-28s%s"%(r['name'], hsv, mesh))
