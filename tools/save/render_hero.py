import bpy, mathutils, math, os, sys
ARGS=sys.argv[sys.argv.index('--')+1:] if '--' in sys.argv else []
OUT=ARGS[0] if ARGS else "/tmp/skater_hero.png"
RES=int(ARGS[1]) if len(ARGS)>1 else 1400
OBJ=os.path.abspath("tools/save/renders/skater_posed.obj")
bpy.ops.wm.read_factory_settings(use_empty=True)
bpy.ops.wm.obj_import(filepath=OBJ, forward_axis='Y', up_axis='Z')
meshes=[o for o in bpy.data.objects if o.type=='MESH']
for o in meshes:
    bpy.context.view_layer.objects.active=o
    for p in o.data.polygons: p.use_smooth=True
    try: bpy.ops.object.shade_smooth_by_angle(angle=math.radians(50))
    except: 
        try: bpy.ops.object.shade_smooth()
        except: pass
for mat in bpy.data.materials:
    if not mat.use_nodes: continue
    nt=mat.node_tree; bsdf=next((n for n in nt.nodes if n.type=='BSDF_PRINCIPLED'),None); img=next((n for n in nt.nodes if n.type=='TEX_IMAGE'),None)
    if img:
        img.interpolation='Cubic'
        if bsdf:
            try: nt.links.new(img.outputs['Alpha'], bsdf.inputs['Alpha'])
            except: pass
    if bsdf:
        try: bsdf.inputs['Roughness'].default_value=0.88
        except: pass
        try: bsdf.inputs['Specular IOR Level'].default_value=0.12
        except: pass
# bbox
mn=mathutils.Vector((1e9,)*3); mx=mathutils.Vector((-1e9,)*3)
for o in meshes:
    for c in o.bound_box:
        w=o.matrix_world@mathutils.Vector(c)
        mn=mathutils.Vector((min(mn.x,w.x),min(mn.y,w.y),min(mn.z,w.z)))
        mx=mathutils.Vector((max(mx.x,w.x),max(mx.y,w.y),max(mx.z,w.z)))
ctr=(mn+mx)/2; size=mx-mn; H=size.z
# camera: 3/4 hero, head-and-shoulders, slightly low
FRAME=os.environ.get('FRAME','head')
cam_d=bpy.data.cameras.new("c"); cam=bpy.data.objects.new("c",cam_d)
bpy.context.scene.collection.objects.link(cam); bpy.context.scene.camera=cam
if FRAME=='banner':
    cam_d.lens=58
    tgt=mathutils.Vector((ctr.x-H*0.22, ctr.y, mn.z+H*0.66))
    cam.location=mathutils.Vector((ctr.x+H*0.30, ctr.y-H*2.6, mn.z+H*0.80))
elif FRAME=='body':
    cam_d.lens=72
    tgt=mathutils.Vector((ctr.x, ctr.y, mn.z+H*0.52))
    cam.location=tgt+mathutils.Vector((H*0.55,-H*2.0,H*0.12))
else:
    cam_d.lens=95
    tgt=mathutils.Vector((ctr.x+H*0.02, ctr.y, mn.z+H*0.85))
    cam.location=tgt+mathutils.Vector((H*0.55,-H*1.0,H*0.0))
cam.rotation_euler=(tgt-cam.location).to_track_quat('-Z','Y').to_euler()
# lighting: warm key + cool rim (separates shirt/hair) + soft fill
def sun(n,loc,energy,color):
    d=bpy.data.lights.new(n,'SUN'); d.energy=energy; d.angle=math.radians(4); d.color=color
    o=bpy.data.objects.new(n,d); bpy.context.scene.collection.objects.link(o)
    o.location=ctr+mathutils.Vector(loc); o.rotation_euler=(ctr-o.location).to_track_quat('-Z','Y').to_euler(); return o
sun("key",(H*0.6,-H*0.8,H*0.7),3.2,(1.0,0.96,0.9))     # warm key (front-right-top)
sun("fill",(-H*0.9,-H*0.4,H*0.1),0.9,(0.85,0.88,1.0))  # cool soft fill (left)
sun("rim",(-H*0.5,H*0.9,H*1.0),6.0,(0.7,0.8,1.0))      # strong cool back-rim
sun("rim2",(H*0.7,H*0.6,H*0.5),3.0,(1.0,0.7,0.9))      # subtle magenta kicker
# dark moody world
w=bpy.data.worlds.new("w"); bpy.context.scene.world=w; w.use_nodes=True
w.node_tree.nodes['Background'].inputs[0].default_value=(0.02,0.02,0.03,1)
w.node_tree.nodes['Background'].inputs[1].default_value=0.25
sc=bpy.context.scene
# engine: Cycles CPU + denoise (quality); fallback EEVEE
try:
    sc.render.engine='CYCLES'; sc.cycles.device='CPU'; sc.cycles.samples=140
    try: sc.cycles.use_denoising=True
    except: pass
except:
    for eng in ('BLENDER_EEVEE_NEXT','BLENDER_EEVEE'):
        try: sc.render.engine=eng; break
        except: pass
try: sc.view_settings.look='Medium High Contrast'
except: pass
try: sc.view_settings.exposure=0.0
except: pass
sc.render.film_transparent=True
if os.environ.get('FRAME')=='banner':
    sc.render.resolution_x=RES; sc.render.resolution_y=int(RES*0.36)
else:
    sc.render.resolution_x=RES; sc.render.resolution_y=RES
sc.render.filepath=OUT
import time; t=time.time()
bpy.ops.render.render(write_still=True)
print("HERO RENDER -> %s (%dx%d, %s, %.0fs)"%(OUT,RES,RES,sc.render.engine,time.time()-t))
