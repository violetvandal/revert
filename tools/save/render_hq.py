import bpy, mathutils, math, os, sys
ARGS=sys.argv[sys.argv.index('--')+1:] if '--' in sys.argv else []
MODE=ARGS[0] if ARGS else 'pfp'   # pfp | banner | full
OBJ=os.path.abspath("tools/save/renders/skater_posed.obj")
OUT=ARGS[1] if len(ARGS)>1 else "/tmp/skater_hq.png"

bpy.ops.wm.read_factory_settings(use_empty=True)
bpy.ops.wm.obj_import(filepath=OBJ, forward_axis='Y', up_axis='Z')
meshes=[o for o in bpy.data.objects if o.type=='MESH']

# --- smooth shading (auto-smooth keeps sharp edges) ---
for o in meshes:
    bpy.context.view_layer.objects.active=o
    for poly in o.data.polygons: poly.use_smooth=True
    try: bpy.ops.object.shade_smooth_by_angle(angle=math.radians(50))
    except Exception:
        try: bpy.ops.object.shade_smooth()
        except: pass

# --- materials: alpha + matte-ish, crisp texture filtering ---
for mat in bpy.data.materials:
    if not mat.use_nodes: continue
    nt=mat.node_tree
    bsdf=next((n for n in nt.nodes if n.type=='BSDF_PRINCIPLED'),None)
    img=next((n for n in nt.nodes if n.type=='TEX_IMAGE'),None)
    if img:
        img.interpolation='Cubic'   # smoother than the blocky default
        if bsdf:
            try: nt.links.new(img.outputs['Alpha'], bsdf.inputs['Alpha'])
            except: pass
    if bsdf:
        try: bsdf.inputs['Roughness'].default_value=0.8
        except: pass
        try: bsdf.inputs['Specular IOR Level'].default_value=0.15
        except: pass
    for a,v in [('blend_method','CLIP'),('shadow_method','CLIP'),('alpha_threshold',0.4),
                ('surface_render_method','DITHERED'),('use_backface_culling',False)]:
        try: setattr(mat,a,v)
        except: pass

# --- bbox ---
mn=mathutils.Vector((1e9,)*3); mx=mathutils.Vector((-1e9,)*3)
for o in meshes:
    for c in o.bound_box:
        w=o.matrix_world@mathutils.Vector(c)
        mn=mathutils.Vector((min(mn.x,w.x),min(mn.y,w.y),min(mn.z,w.z)))
        mx=mathutils.Vector((max(mx.x,w.x),max(mx.y,w.y),max(mx.z,w.z)))
ctr=(mn+mx)/2; size=mx-mn; H=size.z

# --- camera per mode ---
cam_d=bpy.data.cameras.new("c"); cam=bpy.data.objects.new("c",cam_d)
bpy.context.scene.collection.objects.link(cam); bpy.context.scene.camera=cam
cam_d.lens=85
if MODE=='pfp':
    tgt=mathutils.Vector((ctr.x, ctr.y, mn.z+H*0.86))      # head/shoulders
    cam.location=tgt+mathutils.Vector((H*0.18,-H*0.95,H*0.04)); RES=(1600,1600)
elif MODE=='banner':
    # frame head-to-waist, subject shifted left (room for text on the right)
    tgt=mathutils.Vector((ctr.x-H*0.28, ctr.y, mn.z+H*0.80)); cam_d.lens=80
    cam.location=mathutils.Vector((ctr.x+H*0.15, ctr.y-H*2.2, mn.z+H*0.86)); RES=(2400,840)
else:
    tgt=ctr; cam.location=ctr+mathutils.Vector((H*0.35,-H*1.6,H*0.1)); RES=(1400,1900)
cam.rotation_euler=(tgt-cam.location).to_track_quat('-Z','Y').to_euler()

# --- 3-point lighting + subtle world ---
w=bpy.data.worlds.new("w"); bpy.context.scene.world=w; w.use_nodes=True
w.node_tree.nodes['Background'].inputs[1].default_value=0.7
def sun(name,loc,energy):
    d=bpy.data.lights.new(name,'SUN'); d.energy=energy; d.angle=math.radians(3)
    o=bpy.data.objects.new(name,d); bpy.context.scene.collection.objects.link(o)
    o.location=ctr+mathutils.Vector(loc); o.rotation_euler=(ctr-o.location).to_track_quat('-Z','Y').to_euler(); return o
sun("key",(H*0.7,-H*0.9,H*0.7),4.0)
sun("fill",(-H*0.9,-H*0.5,H*0.2),1.6)
sun("rim",(-H*0.2,H*1.0,H*0.9),5.0)

sc=bpy.context.scene
for eng in ('BLENDER_EEVEE_NEXT','BLENDER_EEVEE'):
    try: sc.render.engine=eng; break
    except: pass
try: sc.eevee.taa_render_samples=128
except: pass
sc.render.film_transparent=True
try: sc.view_settings.exposure=0.6
except: pass
try: sc.view_settings.look='Medium High Contrast'
except Exception: pass
sc.render.resolution_x,sc.render.resolution_y=RES
sc.render.filepath=OUT
bpy.ops.render.render(write_still=True)
print("HQ RENDER %s -> %s (%dx%d)"%(MODE,OUT,RES[0],RES[1]))
