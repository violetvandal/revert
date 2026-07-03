#!/usr/bin/env python3
# grfparse: read a THUG2 Create-A-Graphic (.GRF) save file.
# Format (RE'd 2026-06-18): header + a CStruct-serialized {checksumname, layer_infos[10]}.
# Each layer struct = fields keyed by a 1- or 2-byte name id, value tagged by a type byte.
import struct, sys, zlib

def ck(s):  # Neversoft StringToChecksum (CRC32 no final XOR), case-insensitive
    return (zlib.crc32(s.lower().encode('latin1')) ^ 0xffffffff) & 0xffffffff

# field-id -> human name (derived from edit_graphic_copy_layer_infos order + observed ids)
FIELD = {0x01:'texture_id', 0x02:'texture_name', 0x03:'string', 0x04:'canvas_id',
         0x05:'font_id', 0x06:'pos_x', 0x07:'pos_y', 0x08:'rot', 0x25:'scale',
         0x0a:'flip_h', 0x0c:'flip_v', 0x0d:'hsva', 0x0e:'layer_id'}

class R:
    def __init__(s, d, o=0): s.d=d; s.o=o
    def u8(s):  v=s.d[s.o]; s.o+=1; return v
    def u16(s): v=struct.unpack_from('<H',s.d,s.o)[0]; s.o+=2; return v
    def u32(s): v=struct.unpack_from('<I',s.d,s.o)[0]; s.o+=4; return v
    def f32(s): v=struct.unpack_from('<f',s.d,s.o)[0]; s.o+=4; return v
    def cstr(s):
        e=s.d.index(b'\0',s.o); v=s.d[s.o:e]; s.o=e+1; return v.decode('latin1')

def read_field(r):
    """Read one (id,type,value). Returns (fid, typ, value) or None at end-of-struct."""
    t = r.u8()
    if t == 0x00:        # end of struct
        return None
    # name width: 0x40-0x5f types carry a u16 name; 0x80+ types carry a u8 name
    if t < 0x80:
        fid = r.u16()
    else:
        fid = r.u8()
    if   t == 0x4d: val = r.u32()                 # 'M' checksum/name (4 bytes)
    elif t == 0x43: val = r.cstr()                # 'C' string
    elif t == 0x52: val = None                    # 'R' null / default (no data)
    elif t == 0x50: val = r.u8()                  # 'P' int8
    elif t == 0x42: val = r.f32()                 # 'B' float
    elif t == 0x4c:                               # 'L' array
        tag = r.u8(); cnt = r.u8(); pad = r.u8()  # 01 <count> 00
        val = [r.u32() for _ in range(cnt)]
    elif t == 0x82: val = r.f32()                 # float
    elif t == 0x90: val = r.u8()                  # int8
    elif t == 0x91: val = r.u16()                 # int16
    elif t == 0x92: val = 0                        # int 0
    else: raise ValueError("unknown type 0x%02x @0x%x"%(t, r.o-1))
    return (fid, t, val)

def parse(path):
    d = open(path,'rb').read()
    hdr = {'magic': struct.unpack_from('<I',d,0)[0],
           'checksum': struct.unpack_from('<I',d,4)[0],
           'h08': struct.unpack_from('<I',d,8)[0],
           'datalen': struct.unpack_from('<I',d,12)[0],
           'h10': struct.unpack_from('<I',d,16)[0]}
    r = R(d, 0x14)
    # top-level: string field (the graphic name), then an array field of layers
    fields = []
    while r.o < hdr['datalen']:
        f = read_field(r)
        if f is None:
            # could be end of a sub-struct -> keep going until datalen
            continue
        fields.append((f, r.o))
    return d, hdr, fields

if __name__ == '__main__':
    path = sys.argv[1] if len(sys.argv)>1 else 'game-playable-us/Save/VioletVandal.GRF'
    d = open(path,'rb').read()
    hdr = struct.unpack_from('<5I', d, 0)
    print("magic=%08x checksum=%08x h08=%08x datalen=%d(0x%x) h10=%d"%(*hdr[:4],hdr[3],hdr[4]))
    datalen = hdr[3]
    # checksum hunt: CRC32-no-final-xor over candidate ranges
    target = hdr[1]
    for lo in (0x08,0x0c,0x10,0x14):
        for hi in (datalen, len(d), 0x339):
            c = (zlib.crc32(d[lo:hi]) ^ 0xffffffff) & 0xffffffff
            if c == target: print("  CHECKSUM MATCH: CRC-noxor over [0x%x:0x%x]"%(lo,hi))
    # walk top-level
    r = R(d, 0x14)
    # graphic name: 03 <namehash:4> <cstr>
    print("\n-- top-level prefix bytes --")
    print(' '.join('%02x'%b for b in d[0x14:0x2f]))
    # records: split on 4d 01 00
    marker=b'\x4d\x01\x00'
    idxs=[i for i in range(0x14,datalen) if d[i:i+3]==marker]
    print("\n%d layers:"%len(idxs))
    for n,s in enumerate(idxs):
        e = idxs[n+1] if n+1<len(idxs) else datalen
        rr = R(d, s); lay={}
        while rr.o < e:
            f = read_field(rr)
            if f is None: break
            fid,typ,val = f
            lay[FIELD.get(fid,'#%02x'%fid)] = val
        tn = lay.get('texture_name',''); st = lay.get('string','')
        kind = ('text:"%s"'%st) if st else (('sprite:%s'%tn) if tn else 'EMPTY')
        print("  [%d] %-16s pos=(%s,%s) rot=%-7s scale=%-7s flip=%s/%s hsva=%s font=%s lid=%s"%(
            n, kind, lay.get('pos_x'), lay.get('pos_y'),
            ('%.1f'%lay['rot']) if isinstance(lay.get('rot'),float) else lay.get('rot'),
            ('%.3f'%lay['scale']) if isinstance(lay.get('scale'),float) else lay.get('scale'),
            lay.get('flip_h'), lay.get('flip_v'), lay.get('hsva'),
            lay.get('font_id'), lay.get('layer_id')))
