#!/usr/bin/env python3
# Authoring tool for the playas-pro mod: harvest THUG Pro pro/guest skaters into the
# pre-made-skater roster (custom_male_appearances), so they're selectable in stock
# THUG2's main-menu Create-A-Skater. See [[project_thugpro_backport]].
#
#   build_playas_pro.py harvest [thugpro_dir]   # stage guest models + gen cas_skater_m.ns
#   build_playas_pro.py gen                      # regen cas_skater_m.ns from manifest (no THUG Pro)
#
# Two character classes:
#   STOCK pros (Hawk/Burnquist/Koston/Margera/Mullen/Muska/Vallely/Sparrow) already
#     have body parts + appearance structs + models in stock THUG2 -> just a roster line.
#   THUG PRO guests/THUG-pros -> inject model (skin/tex/col -> skaterparts_temp, cas ->
#     casfiles) + add a body part + a minimal appearance struct + a roster line.
import sys, os, re, shutil, glob, zlib, getpass
ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
MOD = os.path.join(ROOT, 'mods', 'src', 'playas-pro')
MODELDIR = os.path.join(MOD, 'models')
MANIFEST = os.path.join(MOD, 'roster.manifest')
PRISTINE = os.path.join(ROOT, 'game-pristine-us', 'Data', 'scripts', 'game', 'cas_skater_m.qb')
NS = os.path.join(ROOT, 'tools', 'neverscript', 'ns')
DEFAULT_TP = os.path.expanduser(f'~/.wine-thugpro/drive_c/users/{getpass.getuser()}/AppData/Local/THUG Pro')

# stock pros: (appearance_struct, display name, voice) -- roster line only, no injection
STOCK = [('appearance_hawk','Tony Hawk','male1'),('appearance_burnquist','Bob Burnquist','male2'),
         ('appearance_koston','Eric Koston','male3'),('appearance_margera','Bam Margera','male4'),
         ('appearance_mullen','Rodney Mullen','male1'),('appearance_muska','Chad Muska','male2'),
         ('appearance_vallely','Mike Vallely','male3'),('appearance_sparrow','Eric Sparrow','male4')]
STOCK_BODIES = {'malebody','femalebody','hawk','burnquist','koston','margera','mullen','muska',
                'vallely','sparrow','shrek','thps_hawk','sk8 hand','nick_secretguy','price_secretguy',
                'call_of_duty','ped_testing','weeman','lasek','sheckler','skaboto'}

def decompile(qb, out):  # stock qb -> clean -o; THUG Pro qb finishes 1 byte short -> stdout
    import subprocess
    if os.path.exists(out): os.remove(out)
    subprocess.run([NS, '-d', qb, '-o', out], capture_output=True)
    if os.path.exists(out) and os.path.getsize(out) > 100:
        return
    r = subprocess.run([NS, '-d', qb], capture_output=True, text=True).stdout
    open(out, 'w').write('\n'.join(l for l in r.split('\n')
                         if 'bytes decompiled' not in l and not l.startswith('next byte:')))

def harvest(tp):
    sys.path.insert(0, os.path.join(ROOT, 'tools', 'prx')); import prx, lzss
    os.makedirs(MODELDIR, exist_ok=True)
    for f in glob.glob(os.path.join(MODELDIR, '*')): os.remove(f)
    ver, ents = prx.parse(open(os.path.join(tp, 'data/pre/thugpro_qb.prx'), 'rb').read())
    e = prx.find(ents, 'qb\\game\\cas_skater_m.qb')
    open('/tmp/_tpcsm.qb', 'wb').write(lzss.decompress(e['blob'][:e['csize']], e['dsize']) if e['csize'] else e['blob'][:e['dsize']])
    decompile('/tmp/_tpcsm.qb', '/tmp/_tpcsm.ns')
    body = re.search(r'\nbody = \[(.*?)\n\] ', open('/tmp/_tpcsm.ns').read(), re.S).group(1)
    md = os.path.join(tp, 'data', 'models')
    rows, seen = [], set()
    voices = ['male1','male2','male3','male4']
    for ent in re.split(r'\n    \{', body):
        did = re.search(r'desc_id=`?([^`\n]+?)`? *\n', ent); me = re.search(r'mesh="([^"]+)"', ent)
        fd = re.search(r'frontend_desc=%"([^"]*)"', ent)
        if not (did and me): continue
        did = did.group(1).strip(); mesh = me.group(1); disp = fd.group(1) if fd else did
        if did.lower() in STOCK_BODIES or did.lower() in seen: continue
        # resolve model files by the mesh basename in its dir
        sub = os.path.dirname(mesh).split('/', 1)[1] if mesh.lower().startswith('models/') else os.path.dirname(mesh)
        base = os.path.basename(mesh)  # e.g. skater_Ironman.skin
        dd = os.path.join(md, *sub.split('/'))
        if not os.path.isdir(dd): continue
        files = {}
        for f in os.listdir(dd):
            for ext in ('.skin.xbx', '.tex.xbx', '.cas.xbx', '.col.xbx'):
                if f.lower() == base.lower().replace('.skin', '') + ext:
                    files[ext] = os.path.join(dd, f)
        if '.skin.xbx' not in files: continue
        seen.add(did.lower())
        # stage each model file with its full archive entry path encoded (\\ -> #)
        for ext, src in files.items():
            staged = '%s.xbx' % base if ext == '.skin.xbx' else base.replace('.skin', ext)
            tgt = 'models\\%s\\%s' % (sub.replace('/', '\\'), staged)
            shutil.copy(src, os.path.join(MODELDIR, tgt.replace('\\', '#')))
        rows.append((did, disp, voices[len(rows) % 4], mesh))
    with open(MANIFEST, 'w') as f:
        f.write('# stock pros (roster only) + harvested THUG Pro guests/pros (model injected)\n')
        for a, n, v in STOCK: f.write('stock\t%s\t%s\t%s\n' % (a, n, v))
        for did, disp, v, mesh in rows: f.write('guest\t%s\t%s\t%s\t%s\n' % (did, disp, v, mesh))
    print('harvested %d guest/pro characters (+%d stock) -> %d model files'
          % (len(rows), len(STOCK), len(os.listdir(MODELDIR))))
    gen()

def gen():
    stock_rows, guest_rows = [], []
    for ln in open(MANIFEST):
        ln = ln.split('#', 1)[0].rstrip('\n')
        if not ln.strip(): continue
        p = re.split(r'\t+', ln.strip())
        if p[0] == 'stock' and len(p) >= 4: stock_rows.append((p[1], p[2], p[3]))
        elif p[0] == 'guest' and len(p) >= 5: guest_rows.append((p[1], p[2], p[3], p[4]))
    decompile(PRISTINE, '/tmp/_stockcsm.ns')
    ns = open('/tmp/_stockcsm.ns').read().split('\n')
    def close_of(name):
        s = next(i for i, l in enumerate(ns) if l.startswith(name + ' = ['))
        return next(i for i in range(s + 1, len(ns)) if ns[i].strip() == ']')
    # guests: body part + appearance struct + roster
    body_c = close_of('body')
    body_block = []
    appearance_block = []
    roster_guest = []
    for did, disp, v, mesh in guest_rows:
        body_block += ['    {', '        desc_id=%s ' % (('`%s`' % did) if ' ' in did else did),
                       '        frontend_desc=%%"%s" ' % disp,
                       '        mesh="%s" ' % mesh, '        hidden ', '    } ']
        appearance_block += ['appearance_vv_%s = {' % re.sub(r'\W', '_', did.lower()),
                             '    body={desc_id=%s} ' % (('`%s`' % did) if ' ' in did else did),
                             '    board={desc_id=`default`} ', '} ']
        roster_guest.append('    {struct=appearance_vv_%s name="%s" voice=%s} '
                            % (re.sub(r'\W', '_', did.lower()), disp, v))
    ns[body_c:body_c] = body_block
    ai = next(i for i, l in enumerate(ns) if l.startswith('custom_male_appearances = ['))
    ns[ai:ai] = appearance_block
    cma_c = close_of('custom_male_appearances')
    roster_stock = ['    {struct=%s name="%s" voice=%s} ' % (a, n, v) for a, n, v in stock_rows]
    ns[cma_c:cma_c] = roster_stock + roster_guest
    out = os.path.join(MOD, 'source', 'cas_skater_m.ns')
    os.makedirs(os.path.dirname(out), exist_ok=True)
    open(out, 'w').write('\n'.join(ns))
    print('generated cas_skater_m.ns: +%d stock pros, +%d guests' % (len(stock_rows), len(roster_guest)))

if __name__ == '__main__':
    cmd = sys.argv[1] if len(sys.argv) > 1 else 'gen'
    if cmd == 'harvest': harvest(sys.argv[2] if len(sys.argv) > 2 else DEFAULT_TP)
    else: gen()
