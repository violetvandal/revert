#!/usr/bin/env python3
# Composite the transparent hero render over a styled (THUG-ish) background + film grain + vignette.
import sys, math
from PIL import Image, ImageDraw, ImageFilter, ImageChops, ImageEnhance
IN=sys.argv[1]; OUT=sys.argv[2]
style=sys.argv[3] if len(sys.argv)>3 else 'purple'
fg=Image.open(IN).convert('RGBA'); W,Hh=fg.size
# --- background: radial gradient (spotlight behind subject) + subtle grunge ---
PAL={'purple':((38,28,52),(14,12,20)),'concrete':((96,96,100),(28,28,30)),
     'teal':((20,52,58),(8,14,18)),'sunset':((90,50,40),(20,14,22))}
c1,c2=PAL.get(style,PAL['purple'])
bg=Image.new('RGB',(W,Hh))
px=bg.load()
cx,cy=W*0.5,Hh*0.42; maxr=math.hypot(W,Hh)*0.6
for y in range(Hh):
    for x in range(0,W,2):
        t=min(1.0,math.hypot(x-cx,y-cy)/maxr)
        r=int(c1[0]*(1-t)+c2[0]*t); g=int(c1[1]*(1-t)+c2[1]*t); b=int(c1[2]*(1-t)+c2[2]*t)
        px[x,y]=(r,g,b); 
        if x+1<W: px[x+1,y]=(r,g,b)
# subtle grunge: blurred noise overlay
import random; random.seed(7)
noise=Image.effect_noise((W//2,Hh//2),28).resize((W,Hh)).convert('L').filter(ImageFilter.GaussianBlur(2))
bg=ImageChops.overlay(bg,Image.merge('RGB',(noise,noise,noise)))
# --- composite character ---
comp=bg.convert('RGBA'); comp.alpha_composite(fg); comp=comp.convert('RGB')
# --- vignette ---
vig=Image.new('L',(W,Hh),0); d=ImageDraw.Draw(vig)
d.ellipse([-W*0.25,-Hh*0.25,W*1.25,Hh*1.25],fill=255); vig=vig.filter(ImageFilter.GaussianBlur(W*0.12))
dark=ImageEnhance.Brightness(comp).enhance(0.45)
comp=Image.composite(comp,dark,vig)
# --- slight contrast/color grade (moody) ---
comp=ImageEnhance.Contrast(comp).enhance(1.12); comp=ImageEnhance.Color(comp).enhance(1.06)
# --- film grain (retro/THUG) ---
g=Image.effect_noise((W,Hh),16).convert('L')
comp=ImageChops.overlay(comp, Image.merge('RGB',(g,g,g))).point(lambda v:v)  # mild
comp=Image.blend(comp, ImageChops.overlay(comp, Image.merge('RGB',(g,g,g))), 0.12)
comp.save(OUT); print("composed %s -> %s"%(style,OUT))
