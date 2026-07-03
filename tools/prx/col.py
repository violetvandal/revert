#!/usr/bin/env python3
"""
THUG2 collision (.col / .col.xbx) v10 tool — parse, inspect, edit face flags/terrain.

Format spec verified against the io_thps_scene Blender addon (Denetii/chc), the
authoritative public reference for THPS .col. THUG2 = version 10, little-endian.

Layout:
  header  32B = <8i: version, num_objects, total_verts, total_large_faces,
                      total_small_faces, total_large_verts, total_small_verts, pad
  objects num_objects * 64B each (SIZEOF_SECTOR_OBJ=64):
    +0x00 u32 checksum   +0x04 u16 flags(mSD_*)   +0x06 u16 num_verts
    +0x08 u16 num_faces  +0x0A u8 use_small_faces  +0x0B u8 use_fixed_verts
    +0x0C u32 first_face_offset  +0x10 4f bbox0  +0x20 4f bbox1
    +0x30 u32 first_vert_offset  +0x34 i32 bsp  +0x38 i32 intensity  +0x3C i32 pad
  base_vert = (32 + 64*num_objects + 15) & ~15            (16-align)
  base_int  = base_vert + large_verts*12 + small_verts*6  (float vert 12B / fixed 6B)
  base_face = (base_int + total_verts + 3) & ~3           (4-align; 1 intensity byte/vert)
  per face: <HH flags, terrain ; then 3H (large, stride 10) or 3B+1pad (small, stride 8)

Terrain enum (u16): SAND=27. mFD_NOT_SKATABLE=0x0002, mFD_SKATABLE=0x0001 (16-bit on disk).

NOTE (THUG2): the AU beach "forced walk on sand" is NOT in collision — it's a level
trigger volume firing AU_EnterSand -> ForceToWalk. Editing .col terrain/flags does
nothing for that. This tool remains useful for genuine collision edits.

Usage:
  col.py info   <file.col.xbx>
  col.py retype <in> <out> <from_terrain> <to_terrain>   # in-place 2-byte terrain edits
  col.py setflag/clrflag <in> <out> <terrain> <flagbits> # set/clear face flag bits on a terrain
"""
import struct, sys, collections

def parse(data):
    h = struct.unpack_from('<8i', data, 0)
    ver, no, tv, lf, sf, lv, sv, _ = h
    assert ver == 10, "not THUG2 col (version=%d)" % ver
    base_vert = (32 + 64*no + 15) & 0xFFFFFFF0
    base_int  = base_vert + lv*12 + sv*6
    base_face = (base_int + tv + 3) & 0xFFFFFFFC
    objs = []
    for i in range(no):
        off = 32 + i*64
        cks, flags, nv, nf = struct.unpack_from('<IHHH', data, off)
        small = data[off+10]; fixed = data[off+11]
        first_face, = struct.unpack_from('<I', data, off+0x0C)
        objs.append(dict(off=off, cks=cks, flags=flags, nv=nv, nf=nf,
                         small=small, fixed=fixed, first_face=first_face))
    return dict(hdr=h, base_face=base_face, objs=objs)

def each_face(data, p):
    """yield (face_offset, flags, terrain) for every face."""
    for o in p['objs']:
        st = 8 if o['small'] else 10
        fb = p['base_face'] + o['first_face']
        for k in range(o['nf']):
            foff = fb + k*st
            fl, tt = struct.unpack_from('<HH', data, foff)
            yield foff, fl, tt

def info(path):
    data = open(path, 'rb').read()
    p = parse(data)
    ver, no, tv, lf, sf, lv, sv, _ = p['hdr']
    print("version=%d objects=%d verts=%d (large=%d small=%d) faces=%d (large=%d small=%d)"
          % (ver, no, tv, lv, sv, lf+sf, lf, sf))
    terr = collections.Counter(); nskat = 0; n = 0
    for _, fl, tt in each_face(data, p):
        terr[tt] += 1; n += 1
        if fl & 2: nskat += 1
    print("faces iterated=%d  not_skatable=%d" % (n, nskat))
    print("terrain histogram:", terr.most_common(20))

def edit(path, out, mode, terrain, arg):
    data = bytearray(open(path, 'rb').read())
    p = parse(data); n = 0
    for foff, fl, tt in each_face(data, p):
        if tt != terrain: continue
        if mode == 'retype':
            struct.pack_into('<H', data, foff+2, arg); n += 1
        elif mode == 'setflag':
            struct.pack_into('<H', data, foff, fl | arg); n += 1
        elif mode == 'clrflag':
            struct.pack_into('<H', data, foff, fl & ~arg); n += 1
    open(out, 'wb').write(data)
    diff = sum(1 for a, b in zip(open(path,'rb').read(), data) if a != b)
    print("%s on terrain %d: %d faces, %d bytes changed -> %s" % (mode, terrain, n, diff, out))

if __name__ == '__main__':
    cmd = sys.argv[1]
    if cmd == 'info':
        info(sys.argv[2])
    elif cmd == 'retype':
        edit(sys.argv[2], sys.argv[3], 'retype', int(sys.argv[4]), int(sys.argv[5]))
    elif cmd in ('setflag', 'clrflag'):
        edit(sys.argv[2], sys.argv[3], cmd, int(sys.argv[4]), int(sys.argv[5], 0))
    else:
        print(__doc__)
