#!/usr/bin/env python3
# extract_skater: ANY THUG2 .SKA save -> build spec (parts, tints, face texture) for the renderer.
# Usage: extract_skater.py <save.SKA> [<gamedir>] [--rebuild]
import os, sys, json, glob
HERE = os.path.dirname(os.path.abspath(__file__)); sys.path.insert(0, HERE)
import caslib

SKIN_PAT = ('head_','face_','_hands','_legs','_lowerlegs','skater_male.skin','skater_female.skin','pvlegs')
def is_skin(mesh): 
    b = mesh.lower()
    return any(p in b for p in SKIN_PAT)
def is_head(mesh):
    b = os.path.basename(mesh).lower()
    return b.startswith('head_') or b.startswith('face_')

def find_file(gamedir, relpath):  # case-insensitive resolve of models/.../x  (+ext)
    parts = relpath.replace('\\','/').split('/'); cur = os.path.join(gamedir,'Data')
    for seg in parts:
        if not os.path.isdir(cur): return None
        nxt = next((x for x in os.listdir(cur) if x.lower()==seg.lower()), None)
        if nxt is None: 
            # last segment: match prefix + extension
            cand = [x for x in os.listdir(cur) if x.lower().startswith(seg.lower()+'.')]
            return os.path.join(cur,cand[0]) if cand else None
        cur = os.path.join(cur,nxt)
    return cur

def build_spec(save, gamedir, rebuild=False):
    cat = caslib.load_catalog(gamedir, rebuild=rebuild)
    s = caslib.parse_save(save, cat); gender = s['gender']
    spec = {'gender': gender, 'save': os.path.basename(save), 'parts': []}
    seen = set()
    def add_mesh(meshrel, tint=None, face_with=None):
        f = find_file(gamedir, meshrel+'.xbx') or find_file(gamedir, meshrel)
        if not f: 
            # try with .skin if missing
            f = find_file(gamedir, meshrel+'.skin.xbx')
        if not f or f in seen: return
        seen.add(f); spec['parts'].append({'mesh': os.path.relpath(f, gamedir), 'tint': tint, 'face_with': face_with})
    # base body meshes for gender
    if gender=='female':
        for m in ['models/skater_female/skater_female.skin','models/skater_female/Skater_F_Legs.skin']: add_mesh(m)
    else:
        for m in ['models/skater_male/skater_male.skin','models/skater_male/Skater_M_Legs.skin']: add_mesh(m)
    # selected parts
    for r in s['selections']:
        if r['name'] in ('None',): continue
        info = cat['parts'].get(r['desc'])
        if not info or not info.get('meshes'): continue
        # gender-appropriate mesh
        meshes = info['meshes']
        pick = next((m for m,g in meshes if g==gender), None) or next((m for m,g in meshes if g=='any'), None) or meshes[0][0]
        if 'skater_pro/' in pick.lower() or 'board' in pick.lower(): continue   # skip boards/pro base
        hsv = None
        if not is_skin(pick) and ('h' in r or 'v' in r) and not (r.get('h',0)==0 and r.get('s',0)==0 and r.get('v',0)==0):
            hsv = [r.get('h',0), r.get('s',0), r.get('v',0)]
        face_with = None
        if is_head(pick) and info.get('with'):
            wbase = os.path.splitext(info['with'])[0]
            wf = find_file(gamedir, wbase+'.img.xbx') or find_file(gamedir, wbase)
            if wf: face_with = os.path.relpath(wf, gamedir)
        add_mesh(pick if pick.endswith('.skin') else pick, hsv, face_with)
    return spec

if __name__=='__main__':
    save=sys.argv[1]; gamedir=sys.argv[2] if len(sys.argv)>2 and not sys.argv[2].startswith('--') else 'game-pristine-us'
    spec=build_spec(save, gamedir, rebuild=('--rebuild' in sys.argv))
    out='/tmp/skater_spec.json'; json.dump(spec, open(out,'w'), indent=1)
    print("gender=%s, %d parts -> %s"%(spec['gender'], len(spec['parts']), out))
    for p in spec['parts']:
        tag=[]
        if p['tint']: tag.append('tint%s'%p['tint'])
        if p['face_with']: tag.append('face='+os.path.basename(p['face_with']))
        print("  %-44s %s"%(p['mesh'], ' '.join(tag)))
