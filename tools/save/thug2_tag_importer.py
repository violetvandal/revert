#!/usr/bin/env python3
"""thug2_tag_importer — turn any image into a Tony Hawk's Underground 2 custom tag.

Non-destructive: writes everything to an OUTPUT folder and never modifies your game
install. You then copy the two produced files into your THUG2 folders (back up first).

  python3 thug2_tag_importer.py myart.png --gamedir "/path/to/THUG2" --name MyTag

Produces in ./out/ :
  <name>.GRF        -> copy into  <THUG2>/Data/Game/Save/   (replacing your tag's .GRF)
  cagpieces.prx     -> copy into  <THUG2>/Data/pre/         (holds the custom sprite)
  <name>_preview.png

The game does not checksum-validate .GRF files, so the result loads directly.
"""
import os, sys, argparse, glob, shutil, zlib, re
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE); sys.path.insert(0, os.path.join(HERE, '..', 'prx'))
import grflib, png2img, prx

def ck(s): return (zlib.crc32(s.lower().encode('latin1')) ^ 0xffffffff) & 0xffffffff

def patch_cagpieces(src_prx_bytes, slot, img_xbx):
    """Return new cagpieces.prx bytes with <slot>.img.xbx replaced by img_xbx."""
    ver, entries = prx.parse(src_prx_bytes)
    target = ('%s.img.xbx' % slot).lower()
    ent = next((e for e in entries
                if e['name'].split(b'\0',1)[0].decode('latin1').lower().replace('/','\\').endswith(target)), None)
    if ent is None:
        raise SystemExit("slot '%s' not found in cagpieces.prx" % slot)
    pad = (-len(img_xbx)) % 4
    ent['dsize'] = len(img_xbx); ent['csize'] = 0; ent['blob'] = img_xbx + b'\0'*pad
    return prx.build(ver, entries)

def full_canvas_tag(graphic_name, slot, scale=1.0):
    """Build a .GRF from scratch: one full-canvas custom sprite (true colour),
    rest empty. The header (cksum1 = StringToChecksum of the name field, h08 =
    name length+7) is computed correctly, so the game accepts an arbitrary name."""
    layer0 = dict(texture_name=slot, string='', font_id=0, pos_x=32, pos_y=32,
                  rot=0.0, scale=float(scale), flip_h=0, flip_v=0, hsva=[0,0,100,128], layer_id=0)
    return grflib.build_grf(graphic_name, [layer0])

def find_cagpieces(gamedir):
    for sub in ('Data/pre', 'data/pre'):
        p = os.path.join(gamedir, sub, 'cagpieces.prx')
        if os.path.exists(p): return p
    raise SystemExit("could not find Data/pre/cagpieces.prx under %s" % gamedir)

def find_existing_tag(gamedir):
    for sub in ('Data/Game/Save', 'Save', 'data/game/save'):
        g = glob.glob(os.path.join(gamedir, sub, '*.GRF')) + glob.glob(os.path.join(gamedir, sub, '*.grf'))
        if g: return g[0]
    return None

def _backup(path):
    """Back up to <path>.orig only if that backup doesn't already exist (preserve true original)."""
    b = path + '.orig'
    if os.path.exists(path) and not os.path.exists(b): shutil.copy(path, b)
    return b if os.path.exists(b) else None

def run(image, gamedir, name=None, slot='grap_50', size=64, scale=1.0, outdir='out',
        preview=True, install=False):
    cag_path = find_cagpieces(gamedir)
    existing = find_existing_tag(gamedir)
    # Build a fresh, correctly-checksummed tag. No pre-existing tag needed: the
    # game's CAG load list shows every .GRF in Save/. Default the tag name to the
    # image's filename (sanitised) so you can literally just point it at an image.
    if name is None:
        base = os.path.splitext(os.path.basename(image))[0]
        name = re.sub(r'[^A-Za-z0-9 ]', '', base).strip()[:15] or 'CustomTag'
    os.makedirs(outdir, exist_ok=True)
    img = png2img.encode(image, size=(size, size))
    new_prx = patch_cagpieces(open(cag_path,'rb').read(), slot, img)
    grf = full_canvas_tag(name, slot, scale)
    out_prx = os.path.join(outdir, 'cagpieces.prx')
    out_grf = os.path.join(outdir, name + '.GRF')
    open(out_prx,'wb').write(new_prx)
    open(out_grf,'wb').write(grf)
    info = {'grf': out_grf, 'prx': out_prx, 'name': name, 'slot': slot,
            'save_dir': os.path.dirname(existing) if existing else '<THUG2>/Data/Game/Save',
            'pre_dir': os.path.dirname(cag_path), 'installed': None}
    if install:
        # cagpieces.prx is shared -> back it up before overwriting
        _backup(cag_path); shutil.copy(out_prx, cag_path)
        # the .GRF is a NEW file named after the tag (shows up in the in-game load
        # list); only back up if a same-named tag already exists
        save_dir = os.path.dirname(existing) if existing else \
                   next((os.path.join(gamedir, s) for s in ('Data/Game/Save','Save','data/game/save')
                         if os.path.isdir(os.path.join(gamedir, s))), os.path.dirname(cag_path))
        dest_grf = os.path.join(save_dir, name + '.GRF')
        if os.path.exists(dest_grf): _backup(dest_grf)
        shutil.copy(out_grf, dest_grf)
        info['installed'] = {'grf': dest_grf, 'prx': cag_path}
    if preview:
        try:
            import grfrender
            xbx = os.path.join(outdir, '_%s.img.xbx'%slot); open(xbx,'wb').write(img)
            info['preview'] = os.path.join(outdir, name+'_preview.png')
            grfrender.render(out_grf, gamedir, info['preview'], SIZE=512, bg=(245,245,248,255),
                             overrides={slot: xbx})
        except Exception as e:
            info['preview'] = 'skipped (%s)'%e
    return info

if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('image', help='source image (PNG/JPG/...)')
    ap.add_argument('--gamedir', required=True, help='your THUG2 install folder')
    ap.add_argument('--name', help="tag name (what shows in the in-game load list; default: existing tag or 'CustomTag')")
    ap.add_argument('--slot', default='grap_50', help='CAGR clip-art slot to commandeer (default grap_50)')
    ap.add_argument('--size', type=int, default=64, choices=[64,128,256], help='sprite resolution')
    ap.add_argument('--scale', type=float, default=1.0)
    ap.add_argument('--out', default='out', help='output folder (default ./out)')
    ap.add_argument('--install', action='store_true',
                    help='copy the files into your THUG2 install automatically (backs up originals to *.orig)')
    a = ap.parse_args()
    i = run(a.image, a.gamedir, a.name, a.slot, a.size, a.scale, a.out, install=a.install)
    print("\n✓ wrote %s\n✓ wrote %s\n✓ preview %s\n" % (i['grf'], i['prx'], i.get('preview')))
    inst = i['installed']
    if inst and inst['grf']:
        print("✓ INSTALLED into your game (originals backed up as *.orig):")
        print("    %s\n    %s" % (inst['grf'], inst['prx']))
        print("\nLoad/spray your tag in-game. Restore anytime by copying the *.orig files back.")
    elif inst and not inst['grf']:
        print("✓ installed cagpieces.prx, but no existing tag .GRF was found to replace.")
        print("  Make any tag in-game once (Create-A-Graphic -> Save), then re-run with --install,")
        print("  or copy %s into %s/ and rename it to match your in-game tag." % (os.path.basename(i['grf']), i['save_dir']))
    else:
        print("INSTALL (your files are untouched; back up before copying):")
        print("  copy  %s  ->  %s/   (it appears in the in-game Create-A-Graphic load list)" % (os.path.basename(i['grf']), i['save_dir']))
        print("  copy  %s  ->  %s/" % ('cagpieces.prx', i['pre_dir']))
        print("\nOr re-run with --install to do this automatically.")
