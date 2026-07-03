#!/usr/bin/env python3
# grf_import: turn an arbitrary PNG into an in-game THUG2 custom tag.
#   1. encode PNG -> .img.xbx (png2img)
#   2. inject it into cagpieces.prx, commandeering a clip-art slot
#   3. build a .GRF with one full-canvas layer referencing that slot (neutral tint)
# The game has no checksum validation on .GRF load (verified), so the result is
# directly loadable: drop the .GRF in Save/ and the modified cagpieces.prx in Data/pre/.
import os, sys, zlib, struct
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE); sys.path.insert(0, os.path.join(HERE,'..','prx'))
import grflib, png2img, prx

def ck(s): return (zlib.crc32(s.lower().encode('latin1')) ^ 0xffffffff) & 0xffffffff
NONE = ck('none')   # 0x806fff30 = "empty layer" sentinel

def inject_sprite(prx_path, slot_basename, img_xbx, out_prx):
    """Replace the cagpieces.prx entry whose basename == <slot>.img.xbx."""
    ver, entries = prx.parse(open(prx_path,'rb').read())
    target = ('%s.img.xbx'%slot_basename).lower()
    ent = None
    for e in entries:
        nm = e['name'].split(b'\0',1)[0].decode('latin1').lower().replace('/','\\')
        if nm.endswith('\\'+target) or nm.endswith(target):
            ent = e; break
    if ent is None: raise RuntimeError("slot %s not found in %s"%(slot_basename, prx_path))
    pad = (-len(img_xbx)) % 4
    ent['dsize']=len(img_xbx); ent['csize']=0; ent['blob']=img_xbx + b'\0'*pad
    open(out_prx,'wb').write(prx.build(ver, entries))
    return ent['name'].split(b'\0',1)[0].decode('latin1')

def clear_layer(parsed, i):
    grflib.set_field(parsed, i, 'texture_id', NONE)
    grflib.set_field(parsed, i, 'texture_name', '')
    grflib.set_field(parsed, i, 'string', '')

def build_custom_tag(template_grf, slot_basename, out_grf, scale=1.0):
    """Make a .GRF: layer 0 = full-canvas custom sprite (neutral tint), rest empty."""
    parsed = grflib.parse(template_grf)
    _, lays = grflib.layers(parsed)
    n = len(lays)
    # layer 0 -> the custom sprite, centred & full-canvas, neutral white tint
    grflib.set_field(parsed, 0, 'texture_name', slot_basename)
    grflib.set_field(parsed, 0, 'texture_id', ck(slot_basename))
    grflib.set_field(parsed, 0, 'string', '')
    grflib.set_field(parsed, 0, 'pos_x', 32)
    grflib.set_field(parsed, 0, 'pos_y', 32)
    grflib.set_field(parsed, 0, 'rot', 0.0)
    grflib.set_field(parsed, 0, 'scale', float(scale))
    grflib.set_field(parsed, 0, 'flip_h', 0)
    grflib.set_field(parsed, 0, 'flip_v', 0)
    for comp,val in (('hue',0),('sat',0),('val',100),('alpha',128)):  # neutral = white tint
        grflib.set_field(parsed, 0, comp, val)
    for i in range(1, n): clear_layer(parsed, i)
    out = grflib.serialize(parsed)
    open(out_grf,'wb').write(out)
    return out

import shutil

def import_png(png, gamedir, grf_name='VioletVandal.GRF', slot='grap_50', size=64, scale=1.0):
    """One-shot: PNG -> in-game custom tag. Auto-backs-up, idempotent (re-injects
    from the pristine cagpieces backup), renders a preview. Returns info dict."""
    import grfrender
    prx_path = os.path.join(gamedir,'Data','pre','cagpieces.prx')
    grf_path = os.path.join(gamedir,'Save', grf_name)
    prx_orig = prx_path + '.orig'; grf_orig = grf_path + '.orig'
    if not os.path.exists(prx_orig): shutil.copy(prx_path, prx_orig)
    if os.path.exists(grf_path) and not os.path.exists(grf_orig): shutil.copy(grf_path, grf_orig)
    template = grf_orig if os.path.exists(grf_orig) else grf_path
    img = png2img.encode(png, size=(size,size))
    xbx_path = os.path.join(os.path.dirname(HERE), 'save', 'grf_work', '%s.img.xbx'%slot)
    os.makedirs(os.path.dirname(xbx_path), exist_ok=True); open(xbx_path,'wb').write(img)
    inject_sprite(prx_orig, slot, img, prx_path)          # from pristine -> live (idempotent)
    build_custom_tag(template, slot, grf_path, scale=scale)
    preview = os.path.join(os.path.dirname(HERE), 'save', 'renders', 'custom_tag_preview.png')
    grfrender.render(grf_path, gamedir, preview, SIZE=512, bg=(245,245,248,255),
                     overrides={slot: xbx_path})
    return {'img_bytes': len(img), 'slot': slot, 'grf': grf_path, 'prx': prx_path, 'preview': preview}

if __name__ == '__main__':
    import argparse
    ap = argparse.ArgumentParser(description="Turn a PNG into a THUG2 custom in-game tag.")
    ap.add_argument('png'); ap.add_argument('gamedir', nargs='?', default='game-playable-us')
    ap.add_argument('--grf', default='VioletVandal.GRF', help="target tag file in <gamedir>/Save")
    ap.add_argument('--slot', default='grap_50', help="CAGR clip-art slot to commandeer")
    ap.add_argument('--size', type=int, default=64, choices=[64,128,256])
    ap.add_argument('--scale', type=float, default=1.0)
    a = ap.parse_args()
    r = import_png(a.png, a.gamedir, a.grf, a.slot, a.size, a.scale)
    print("✓ encoded %d-byte sprite into slot '%s'\n✓ wrote tag %s\n✓ preview %s\n  (originals backed up as *.orig)"
          % (r['img_bytes'], r['slot'], r['grf'], r['preview']))
