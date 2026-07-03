#!/usr/bin/env python3
# grfrender: render a THUG2 .GRF tag to a PNG preview.
# Composites the clip-art sprites + per-letter font text exactly as the editor's
# edit_graphic_prepare_sprite_infos / prepare_text_sprite_infos lay them out:
#   canvas = 64 units, sprite centred at (pos-32), footprint = 64*scale units,
#   rotated by `rot` deg, tinted by HSVtoRGB(hsva).  z-order = layer_id.
import os, sys, math, colorsys, glob
from PIL import Image
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import grflib, img2png

FONT_NAMES = ['graf1','graf2','bold1','block1','ns1','sten1','spr1','threed']
FONT_SPACING = [1, 0.8, 0.65, 0.85, 0.82, 0.65, 0.8, 0.45]

def build_image_index(gamedir):
    """filename(lower, no ext) -> path, over Data/images/CAGR/**.img.xbx"""
    idx = {}
    root = os.path.join(gamedir, 'Data', 'images', 'CAGR')
    for p in glob.glob(os.path.join(root, '**', '*.img.xbx'), recursive=True):
        base = os.path.basename(p)[:-len('.img.xbx')].lower()
        idx.setdefault(base, p)
    return idx

_cache = {}
def load_sprite(idx, name):
    name = name.lower()
    if name in _cache: return _cache[name]
    p = idx.get(name)
    if not p: _cache[name]=None; return None
    w,h,rgba = img2png.decode_img(p)
    im = Image.frombytes('RGBA',(w,h),rgba)
    # NOTE: do NOT flip sprites individually. The whole CAGR canvas is Y-mirrored
    # (canvas Y inverted + textures stored Y-flipped); we composite in engine
    # space and flip the ENTIRE canvas once at the end so positions, rotations
    # and glyph orientation all stay consistent.
    _cache[name]=im; return im

def tint(im, hsva):
    """Multiply the sprite by HSVtoRGB(hsva) — matches the engine's SpriteElement
    rgba multiply. Grey silhouettes -> hue; full-colour sprites -> true colour at
    neutral (s=0,v=100) white tint. hsva=[h0-360,s0-100,v0-100,a0-128]."""
    import numpy as np
    h,s,v,a = (list(hsva)+[0,0,100,128])[:4]
    cr,cg,cb = colorsys.hsv_to_rgb((h%360)/360.0, min(s,100)/100.0, min(v,100)/100.0)
    amul = max(0.0, min(1.0, a/128.0))
    arr = np.asarray(im, dtype=np.float32).copy()
    arr[...,0]*=cr; arr[...,1]*=cg; arr[...,2]*=cb; arr[...,3]*=amul
    return Image.fromarray(arr.clip(0,255).astype('uint8'), 'RGBA')

def paste_layer(canvas, sprite, pos_x, pos_y, scale, rot, flip_h, flip_v, hsva, PXU, SIZE):
    """Place one already-loaded sprite. pos in 0..64 canvas units (we shift to centre)."""
    if sprite is None: return
    foot = max(1, int(round(64*scale*PXU)))   # footprint px (64-unit source * scale)
    im = sprite.resize((foot,foot), Image.LANCZOS)
    if flip_h: im = im.transpose(Image.FLIP_LEFT_RIGHT)
    if flip_v: im = im.transpose(Image.FLIP_TOP_BOTTOM)
    im = tint(im, hsva)
    if rot: im = im.rotate(rot, resample=Image.BICUBIC, expand=True)
    # canvas centre = SIZE/2; pos already centred (pos-32) -> * PXU
    cx = SIZE/2 + pos_x*PXU
    cy = SIZE/2 - pos_y*PXU                     # +y up
    canvas.alpha_composite(im, (int(cx-im.width/2), int(cy-im.height/2)))

def render(grf_path, gamedir, out_path, SIZE=512, bg=None, overrides=None):
    idx = build_image_index(gamedir)
    if overrides:                       # name -> path, to preview injected custom sprites
        idx.update({k.lower():v for k,v in overrides.items()})
        _cache.clear()
    parsed = grflib.parse(grf_path)
    _, lays = grflib.layers(parsed)
    PXU = SIZE/64.0                            # px per canvas unit
    canvas = Image.new('RGBA',(SIZE,SIZE),(0,0,0,0) if bg is None else bg)
    # order by layer_id (0 = bottom)
    def lid(l): return l.get('layer_id',0) or 0
    for l in sorted(lays, key=lid):
        tn = (l.get('texture_name') or '').strip()
        st = (l.get('string') or '')
        hsva = list(l.get('hsva',('ARR',1,4,[0,0,100,128]))[3])
        rot = l.get('rot',0) or 0
        if isinstance(rot,int): rot = float(rot)
        scale = l.get('scale',1.0)
        if isinstance(scale,int): scale = float(scale)
        fh = (l.get('flip_h',0) or 0)==1; fv=(l.get('flip_v',0) or 0)==1
        if tn:                                  # ---- sprite layer ----
            sp = load_sprite(idx, tn)
            paste_layer(canvas, sp, l['pos_x']-32, l['pos_y']-32, scale, rot, fh, fv, hsva, PXU, SIZE)
        elif st:                                # ---- text layer ----
            fid = l.get('font_id',0) or 0
            font = FONT_NAMES[fid] if fid<len(FONT_NAMES) else 'graf1'
            spacing = FONT_SPACING[fid] if fid<len(FONT_SPACING) else 1
            n = len(st)
            cis = (64.0/n)*scale                # char image size (canvas units)
            cscale = cis/64.0
            if n!=1 and spacing!=0: cscale *= (1/spacing)
            cxp = -(cis*n)/2 + cis/2
            for ch in st:
                if ch != ' ':
                    name = '%s_%s'%(font, ch.lower())
                    sp = load_sprite(idx, name)
                    # rotate the baseline offset by rot (Get2DPosFrom2DVec)
                    th = math.radians(rot)
                    ox = cxp*math.cos(th); oy = cxp*math.sin(th)
                    paste_layer(canvas, sp, (l['pos_x']-32)+ox, (l['pos_y']-32)+oy,
                                cscale, rot, fh, fv, hsva, PXU, SIZE)
                cxp += cis
    # whole-canvas Y-mirror: corrects glyph orientation + positions + rotation
    # direction together (the CAGR canvas is Y-inverted relative to screen).
    canvas = canvas.transpose(Image.FLIP_TOP_BOTTOM)
    if bg is not None:
        canvas = canvas.convert('RGB')
    canvas.save(out_path)
    print("rendered %d layers -> %s (%dx%d)"%(len(lays), out_path, SIZE, SIZE))

if __name__ == '__main__':
    grf = sys.argv[1] if len(sys.argv)>1 else 'game-playable-us/Save/VioletVandal.GRF'
    game = sys.argv[2] if len(sys.argv)>2 else 'game-pristine-us'
    out = sys.argv[3] if len(sys.argv)>3 else 'tools/save/renders/tag_VioletVandal.png'
    os.makedirs(os.path.dirname(out), exist_ok=True)
    render(grf, game, out)
