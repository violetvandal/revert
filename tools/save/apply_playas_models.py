#!/usr/bin/env python3
# Inject the playas-pro guest/pro skater models into the install. The script half
# (cas_skater_m.qb) is a normal ns-inject mod; the MODELS must live in the prx the
# game loads skater meshes from: .cas.xbx -> casfiles.prx, .skin/.tex/.col ->
# skaterparts_temp.prx. Staged model files in the mod's models/ dir are named with
# their full archive entry path, '\\' encoded as '#'. Idempotent (skips existing).
#
# Usage: apply_playas_models.py <gamedir> <models-dir>
import sys, os, glob, zlib
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'prx'))
import prx, lzss

def ck(nb): return (zlib.crc32(nb.lower()) ^ 0xffffffff) & 0xffffffff

def add(prxpath, entry_name, raw, have):
    if entry_name.lower() in have: return False
    comp = lzss.compress(raw); assert lzss.decompress(comp, len(raw)) == raw
    name = entry_name.encode('latin1'); nlen = (len(name) + 1 + 3) & ~3
    return {'dsize': len(raw), 'csize': len(comp), 'nlen': nlen, 'crc': ck(name),
            'name': name, 'blob': comp + b'\0' * (((len(comp) + 3) & ~3) - len(comp))}

def apply(gamedir, modeldir):
    buckets = {}   # prx path -> list of staged files
    for f in sorted(glob.glob(os.path.join(modeldir, '*'))):
        entry = os.path.basename(f).replace('#', '\\')
        arc = 'casfiles.prx' if entry.lower().endswith('.cas.xbx') else 'skaterparts_temp.prx'
        buckets.setdefault(arc, []).append((entry, f))
    for arc, items in buckets.items():
        path = os.path.join(gamedir, 'Data', 'pre', arc)
        if not os.path.exists(path): print('  missing', arc); continue
        ver, en = prx.parse(open(path, 'rb').read())
        have = {e['name'].split(b'\0', 1)[0].decode('latin1').lower() for e in en}
        added = 0
        for entry, f in items:
            e = add(path, entry, open(f, 'rb').read(), have)
            if e: en.append(e); have.add(entry.lower()); added += 1
        if added: open(path, 'wb').write(prx.build(ver, en))
        print('  %-22s +%d models (%d already present)' % (arc, added, len(items) - added))

if __name__ == '__main__':
    if len(sys.argv) < 3: sys.exit('usage: apply_playas_models.py <gamedir> <models-dir>')
    apply(sys.argv[1], sys.argv[2])
