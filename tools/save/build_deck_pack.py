#!/usr/bin/env python3
# Authoring tool for the decks-pack mod: harvest THUG Pro deck graphics into the
# mod, and (re)generate source/decks.ns from the manifest + pristine stock decks.
#
#   build_deck_pack.py harvest [thugpro_dir]   # pull new decks from THUG Pro -> textures/ + manifest
#   build_deck_pack.py gen                      # regen source/decks.ns from manifest + pristine (no THUG Pro needed)
#
# The mod's source/decks.ns is committed/static (rebuild compiles it directly); this
# tool just makes scaling the deck list convenient. See [[project_thugpro_backport]].
import sys, os, re, shutil, glob, getpass

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
MOD  = os.path.join(ROOT, 'mods', 'src', 'decks-pack')
TEXDIR = os.path.join(MOD, 'textures')
MANIFEST = os.path.join(MOD, 'decks.manifest')
PRISTINE_DECKS = os.path.join(ROOT, 'game-pristine-us', 'Data', 'scripts', 'game', 'decks.qb')
NS = os.path.join(ROOT, 'tools', 'neverscript', 'ns')
DEFAULT_TP = os.path.expanduser(f'~/.wine-thugpro/drive_c/users/{getpass.getuser()}/AppData/Local/THUG Pro')

def safe(s):  # vv texture name from a path/desc
    return 'vv_' + re.sub(r'[^a-z0-9_]', '_', os.path.basename(s).lower())

def parse_list(ns_text, listname):
    m = re.search(r'\n%s = \[(.*?)\n\] ' % listname, ns_text, re.S)
    out = []
    if not m:
        return out
    for ent in re.split(r'\n    \{', m.group(1)):
        did = re.search(r'desc_id=`?([^`\n]+?)`? *\n', ent)
        fd  = re.search(r'frontend_desc=%"([^"]*)"', ent)
        wi  = re.search(r'with="([^"]+)"', ent)
        if did:
            has = 'common_deck_graphic_params' in ent or 'common_griptape_params' in ent
            out.append((did.group(1).strip(), fd.group(1) if fd else '',
                        wi.group(1) if wi else '', has))
    return out

def harvest(tp):
    os.makedirs(TEXDIR, exist_ok=True)
    for f in glob.glob(os.path.join(TEXDIR, '*.img.xbx')): os.remove(f)
    # decompile both deck lists
    os.system('"%s" -d "%s" -o /tmp/_stock_decks.ns >/dev/null 2>&1' % (NS, PRISTINE_DECKS))
    tp_qb = '/tmp/_tp_decks.qb'
    import subprocess, sys as _s
    # extract THUG Pro decks.qb from its prx
    sys.path.insert(0, os.path.join(ROOT, 'tools', 'prx')); import prx, lzss
    ver, ents = prx.parse(open(os.path.join(tp, 'data/pre/thugpro_qb.prx'), 'rb').read())
    e = prx.find(ents, 'qb\\game\\decks.qb')
    open(tp_qb, 'wb').write(lzss.decompress(e['blob'][:e['csize']], e['dsize']) if e['csize'] else e['blob'][:e['dsize']])
    # THUG Pro's qb decompiles ~99.97% (1 byte short) so `-o` won't write a file —
    # it prints to stdout instead; capture and strip the status footer lines.
    raw = subprocess.run([NS, '-d', tp_qb], capture_output=True, text=True).stdout
    clean = '\n'.join(l for l in raw.split('\n')
                      if 'bytes decompiled' not in l and not l.startswith('next byte:'))
    open('/tmp/_tp_decks.ns', 'w').write(clean)
    stock = parse_list(open('/tmp/_stock_decks.ns').read(), 'deck_graphic')
    tpd   = parse_list(open('/tmp/_tp_decks.ns').read(), 'deck_graphic')
    stock_ids = {e[0].lower() for e in stock}
    def find_tex(withpath):
        base = os.path.basename(withpath)
        for d in ['data/textures/custom_boards', 'data/textures/board_textures', 'data/textures/boards']:
            dd = os.path.join(tp, d)
            if os.path.isdir(dd):
                for f in os.listdir(dd):
                    if f.lower() == base.lower() + '.img.xbx' or f.lower() == base.lower() + '.tex.xbx':
                        return os.path.join(dd, f)
        return None
    stock_grip = parse_list(open('/tmp/_stock_decks.ns').read(), 'griptape')
    tp_grip    = parse_list(open('/tmp/_tp_decks.ns').read(), 'griptape')
    stock_grip_ids = {e[0].lower() for e in stock_grip}
    rows = []
    def take(entries, kind, paramflag, known):
        for did, fd, wi, hasflag in entries:
            if did.lower() in known or not hasflag or not wi.startswith('textures/'):
                continue
            src = find_tex(wi)
            if not src:
                continue
            vv = safe(wi)
            shutil.copy(src, os.path.join(TEXDIR, vv + '.img.xbx'))
            rows.append((kind, vv, 'VV ' + (fd or did)[:20], (fd or did)[:24]))
    take(tpd, 'deck', 'common_deck_graphic_params', stock_ids)
    take(tp_grip, 'grip', 'common_griptape_params', stock_grip_ids)
    with open(MANIFEST, 'w') as f:
        f.write('# <type>\t<vv_texture_basename>\t<desc_id>\t<frontend_desc>  (harvested from THUG Pro)\n')
        for kind, vv, did, disp in rows:
            f.write('%s\t%s\t%s\t%s\n' % (kind, vv, did, disp))
    print('harvested %d entries (%d deck, %d grip)' % (
        len(rows), sum(r[0] == 'deck' for r in rows), sum(r[0] == 'grip' for r in rows)))
    return rows

def gen():
    decks, grips = [], []
    for ln in open(MANIFEST):
        ln = ln.split('#', 1)[0].rstrip('\n')
        if not ln.strip(): continue
        p = re.split(r'\t+', ln.strip())
        # supports 4-col (type vv did disp) or legacy 3-col (vv did disp -> deck)
        if len(p) >= 4:   kind, vv, did, disp = p[0], p[1], p[2], p[3]
        elif len(p) == 3: kind, vv, did, disp = 'deck', p[0], p[1], p[2]
        else: continue
        (grips if kind == 'grip' else decks).append((vv, did, disp))
    os.system('"%s" -d "%s" -o /tmp/_stock_decks.ns >/dev/null 2>&1' % (NS, PRISTINE_DECKS))
    lines = open('/tmp/_stock_decks.ns').read().split('\n')
    def entry(vv, did, disp, param):
        return ('    {\n        desc_id=`%s` \n        frontend_desc=%%"%s" \n'
                '        %s \n        with="textures/boards/%s" \n    } ' % (did, disp, param, vv))
    def close_of(listname):
        s = next(i for i, l in enumerate(lines) if l.startswith(listname + ' = ['))
        return next(i for i in range(s + 1, len(lines)) if lines[i].strip() == ']')
    # insert griptape (later in file) FIRST so deck_graphic's index stays valid
    inserts = [('griptape', grips, 'common_griptape_params'),
               ('deck_graphic', decks, 'common_deck_graphic_params')]
    inserts.sort(key=lambda x: close_of(x[0]), reverse=True)
    for listname, rows, param in inserts:
        if not rows: continue
        c = close_of(listname)
        block = [entry(vv, did, disp, param) for vv, did, disp in rows]
        lines[c:c] = block
    out = os.path.join(MOD, 'source', 'decks.ns')
    os.makedirs(os.path.dirname(out), exist_ok=True)
    open(out, 'w').write('\n'.join(lines))
    print('generated source/decks.ns: +%d decks, +%d griptapes' % (len(decks), len(grips)))

if __name__ == '__main__':
    cmd = sys.argv[1] if len(sys.argv) > 1 else 'gen'
    if cmd == 'harvest':
        harvest(sys.argv[2] if len(sys.argv) > 2 else DEFAULT_TP); gen()
    else:
        gen()
