#!/usr/bin/env python3
# Inject extra Create-A-Skater deck-graphic textures into skaterparts.prx.
#
# The decks-pack mod has two halves:
#   - SCRIPT: source/decks.ns -> qb_scripts.prx (handled by the canonical ns-inject
#     pipeline / thugkit, since decks.qb is a normal script).
#   - TEXTURES (this tool): the .img.xbx deck art must live INSIDE skaterparts.prx
#     (the game ignores loose Data/textures for skater assets — proven by the panty
#     dig, see [[project_cas_texture_modding]]). thugkit's prx op only REPLACES
#     existing entries; here we ADD new ones, so we do it directly.
#
# Each texture <name>.img.xbx is added as entry `textures\boards\<name>.img.xbx`.
# The deck entry in decks.qb references it via with="textures/boards/<name>".
# Idempotent: entries already present are skipped (safe to re-run every rebuild).
#
# Usage: apply_deck_pack.py <gamedir> <texture-dir>
import sys, os, glob, zlib
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'prx'))
import prx, lzss

def ck(name_bytes):
    return (zlib.crc32(name_bytes.lower()) ^ 0xffffffff) & 0xffffffff

def apply(gamedir, texdir):
    prx_path = os.path.join(gamedir, 'Data', 'pre', 'skaterparts.prx')
    if not os.path.exists(prx_path):
        print('  skaterparts.prx missing — skip deck textures'); return
    ver, entries = prx.parse(open(prx_path, 'rb').read())
    have = {e['name'].split(b'\0', 1)[0].decode('latin1').lower() for e in entries}
    added = skipped = 0
    for f in sorted(glob.glob(os.path.join(texdir, '*.img.xbx'))):
        base = os.path.basename(f)
        entry_name = 'textures\\boards\\' + base
        if entry_name.lower() in have:
            skipped += 1; continue
        raw = open(f, 'rb').read()
        comp = lzss.compress(raw)
        assert lzss.decompress(comp, len(raw)) == raw, 'lzss roundtrip ' + base
        name = entry_name.encode('latin1')
        nlen = (len(name) + 1 + 3) & ~3            # null-terminated, 4-byte aligned
        entries.append({
            'dsize': len(raw), 'csize': len(comp), 'nlen': nlen, 'crc': ck(name),
            'name': name, 'blob': comp + b'\0' * (((len(comp) + 3) & ~3) - len(comp)),
        })
        added += 1
    if added:
        open(prx_path, 'wb').write(prx.build(ver, entries))
    print('  deck textures: +%d added, %d already present' % (added, skipped))

if __name__ == '__main__':
    if len(sys.argv) < 3:
        sys.exit('usage: apply_deck_pack.py <gamedir> <texture-dir>')
    apply(sys.argv[1], sys.argv[2])
