#!/usr/bin/env python3
# grflib: read/write THUG2 Create-A-Graphic (.GRF) tag files.
# Format (RE'd 2026-06-18, byte-exact round-trip verified):
#   header (0x14 bytes): u32 cksum0, u32 cksum1, u32 h08, u32 datalen, u32 h10
#   then a CStruct token stream [0x14 : datalen]: a graphic-name field + 10 layer structs.
#   token = <type:u8> <name> <value>; name is u16 LE for type<0x80, else u8.
#   trailer: file padded to 0x8000 (32768) with 0x69 ('i').
import struct, zlib, sys

PAD = 0x69
FILESIZE = 0x8000

def ck(s):  # Neversoft StringToChecksum (CRC32 no final XOR, case-insensitive)
    return (zlib.crc32(s.lower().encode('latin1')) ^ 0xffffffff) & 0xffffffff

FIELD = {0x01:'texture_id', 0x02:'texture_name', 0x03:'string', 0x04:'canvas_id',
         0x05:'font_id', 0x06:'pos_x', 0x07:'pos_y', 0x08:'rot', 0x25:'scale',
         0x0a:'flip_h', 0x0c:'flip_v', 0x0d:'hsva', 0x0e:'layer_id'}

class Rd:
    def __init__(s, d, o=0): s.d=d; s.o=o
    def u8(s):  v=s.d[s.o]; s.o+=1; return v
    def u16(s): v=struct.unpack_from('<H',s.d,s.o)[0]; s.o+=2; return v
    def u32(s): v=struct.unpack_from('<I',s.d,s.o)[0]; s.o+=4; return v
    def f32(s): v=struct.unpack_from('<f',s.d,s.o)[0]; s.o+=4; return v
    def cstr(s):
        e=s.d.index(b'\0',s.o); v=s.d[s.o:e]; s.o=e+1; return v.decode('latin1')

# type byte = NAME-WIDTH flag | BASE type
#   flag: 0x00 -> u32 checksum name ; 0x40 -> u16 name ; 0x80 -> u8 name
#   base: 0x02 float, 0x03 string, 0x0a struct, 0x0c array, 0x0d name(checksum),
#         0x10 int(1 byte), 0x11 int(2 byte), 0x12 int(=0, no bytes)
def _name_width(t): return {0x00:4, 0x40:2, 0x80:1}[t & 0xc0]
def _base(t): return t & 0x3f

def _read_array(r):
    """Array header: <elemtype:u8><count:u16>. Scalar arrays are read inline;
    struct arrays (elemtype 0x0a) are header-only (elements follow as tokens)."""
    elemtype = r.u8(); count = r.u16()
    eb = _base(elemtype)
    if eb == 0x0a:                       # array of structs: header only
        return ('ARR_STRUCT', elemtype, count)
    vals = []
    for _ in range(count):
        if   eb == 0x01: vals.append(r.u32())
        elif eb == 0x02: vals.append(r.f32())
        elif eb == 0x0d: vals.append(r.u32())
        elif eb == 0x10: vals.append(r.u8())
        elif eb == 0x11: vals.append(r.u16())
        else: raise ValueError("array elemtype 0x%02x unhandled @0x%x"%(elemtype, r.o))
    return ('ARR', elemtype, count, vals)

def read_token(r):
    """Returns (type, name, value) or ('END',None,None). Preserves the exact type tag."""
    t = r.u8()
    if t == 0x00: return (0x00, None, None)
    nw = _name_width(t)
    name = r.u32() if nw==4 else (r.u16() if nw==2 else r.u8())
    b = _base(t)
    if   b == 0x02: v = r.f32()
    elif b == 0x03: v = r.cstr()
    elif b == 0x0d: v = r.u32()
    elif b == 0x0a: v = ('STRUCT',)          # struct header; fields follow until END
    elif b == 0x0c: v = _read_array(r)
    elif b == 0x10: v = r.u8()
    elif b == 0x11: v = r.u16()
    elif b == 0x12: v = 0
    else: raise ValueError("unknown base 0x%02x (type 0x%02x) @0x%x"%(b, t, r.o-1))
    return (t, name, v)

def write_token(out, t, name, v):
    out.append(t)
    if t == 0x00: return
    nw = _name_width(t)
    out += struct.pack('<I',name) if nw==4 else (struct.pack('<H',name) if nw==2 else bytes([name]))
    b = _base(t)
    if   b == 0x02: out += struct.pack('<f', v)
    elif b == 0x03: out += v.encode('latin1') + b'\0'
    elif b == 0x0d: out += struct.pack('<I', v)
    elif b == 0x0a: pass
    elif b == 0x0c:
        if v[0] == 'ARR_STRUCT':
            _, elemtype, count = v; out += bytes([elemtype]) + struct.pack('<H', count)
        else:
            _, elemtype, count, vals = v
            out += bytes([elemtype]) + struct.pack('<H', count)
            eb = _base(elemtype)
            for x in vals:
                if   eb in (0x01,0x0d): out += struct.pack('<I', x)
                elif eb == 0x02: out += struct.pack('<f', x)
                elif eb == 0x10: out += bytes([x])
                elif eb == 0x11: out += struct.pack('<H', x)
    elif b == 0x10: out += bytes([v])
    elif b == 0x11: out += struct.pack('<H', v)
    elif b == 0x12: pass
    else: raise ValueError("can't write base 0x%02x"%b)

def parse(path_or_bytes):
    d = path_or_bytes if isinstance(path_or_bytes,(bytes,bytearray)) else open(path_or_bytes,'rb').read()
    d = bytes(d)
    hdr = list(struct.unpack_from('<5I', d, 0))
    datalen = hdr[3]
    r = Rd(d, 0x14)
    tokens = []                      # flat token list for exact round-trip
    while r.o < datalen:
        tok = read_token(r)
        tokens.append(tok)
        if r.o >= datalen: break
    return {'hdr': hdr, 'tokens': tokens, 'raw': d}

def serialize(parsed):
    """Re-emit the file bytes from a parsed dict (header preserved verbatim)."""
    out = bytearray()
    for (t,name,v) in parsed['tokens']:
        write_token(out, t, name, v)
    body = bytes(out)
    hdr = list(parsed['hdr'])
    datalen = 0x14 + len(body)
    hdr[3] = datalen
    head = struct.pack('<5I', *hdr)
    full = head + body
    full += bytes([PAD]) * (FILESIZE - len(full))
    return bytes(full)

# ---- from-scratch builder (no template needed) ----
GRAPHIC_NAME_FIELD = 0xc3f4169a          # field-name checksum of the graphic-name string
LAYER_ARRAY_FIELD  = ck('layer_infos')   # = 0x9a5ff0a3

def _emit_layer(lay, idx):
    """Emit the canonical token sequence for one layer struct (+ END)."""
    tn = (lay.get('texture_name') or '')
    st = (lay.get('string') or '')
    tid = ck(tn) if tn else ck('none')
    fid = int(lay.get('font_id', 0) or 0)
    fh  = 1 if lay.get('flip_h') else 0
    fv  = 1 if lay.get('flip_v') else 0
    lid = int(lay.get('layer_id', idx))
    hsva = list(lay.get('hsva', [0,0,100,128]))
    def i8(name, v): return (0x50, name, v) if v else (0x52, name, 0)
    return [
        (0x4d, 0x01, tid),
        (0x43, 0x02, tn),
        (0x43, 0x03, st),
        (0x4d, 0x04, lay.get('canvas_id') or ck('cag_canvas_%d'%idx)),
        i8(0x05, fid),
        (0x50, 0x06, int(lay.get('pos_x', 32))),
        (0x50, 0x07, int(lay.get('pos_y', 32))),
        (0x42, 0x08, float(lay.get('rot', 0.0))),       # rot always float
        (0x82, 0x25, float(lay.get('scale', 1.0))),     # scale always float (u8-name field)
        i8(0x0a, fh),
        i8(0x0c, fv),
        (0x4c, 0x0d, ('ARR', 0x01, 4, hsva)),
        i8(0x0e, lid),
        (0x00, None, None),
    ]

def name_checksum(graphic_name):
    """The header cksum1 (offset 4) the editor validates: StringToChecksum
    (CRC32 EDB88320, init 0xFFFFFFFF, no final XOR) over the serialized name
    field bytes  03 9a16f4c3 <name> 00 00.  Cracked from game-saved samples."""
    region = bytes([0x03,0x9a,0x16,0xf4,0xc3]) + graphic_name.encode('latin1') + b'\0\0'
    return (zlib.crc32(region) ^ 0xffffffff) & 0xffffffff

def build_grf(graphic_name, layers, h10=1):
    """Construct a complete, game-loadable .GRF from scratch (correct header).
    `layers` = list of up to 10 dicts (keys: texture_name, string, font_id,
    pos_x, pos_y, rot, scale, flip_h, flip_v, hsva[4], layer_id). Padded to 10."""
    layers = [dict(l) for l in layers][:10]
    while len(layers) < 10: layers.append({})
    for i,l in enumerate(layers): l.setdefault('layer_id', i)
    tokens = [(0x03, GRAPHIC_NAME_FIELD, graphic_name),
              (0x00, None, None),
              (0x0c, LAYER_ARRAY_FIELD, ('ARR_STRUCT', 0x0a, 10))]
    for i,l in enumerate(layers): tokens += _emit_layer(l, i)
    body = bytearray()
    for (t,name,v) in tokens: write_token(body, t, name, v)
    cksum1 = name_checksum(graphic_name)                 # validated by the editor
    cksum0 = (zlib.crc32(bytes(body)) ^ 0xffffffff) & 0xffffffff   # body checksum (not validated; just non-zero)
    h08 = len(graphic_name) + 7                          # name-field byte length
    return serialize({'hdr': [cksum0, cksum1, h08, 0, h10], 'tokens': tokens, 'raw': b''})

# ---- editing API ----
NAME2FIELD = {v:k for k,v in FIELD.items()}

def set_field(parsed, layer_index, field, value):
    """Update a field of layer `layer_index` (0-based, by position) in place.
    `field` is a name from FIELD (e.g. 'pos_x','rot','string') or 'hue'/'sat'/'val'/'alpha'
    for individual hsva components. Returns True if updated."""
    fid = NAME2FIELD.get(field)
    hsva_comp = {'hue':0,'sat':1,'val':2,'alpha':3}.get(field)
    cur = -1
    for i,(t,name,v) in enumerate(parsed['tokens']):
        if t == 0x4d and name == 0x01:
            cur += 1
        if cur != layer_index: continue
        if hsva_comp is not None and name == NAME2FIELD['hsva']:
            arr = list(v); arr[3][hsva_comp] = int(value)
            parsed['tokens'][i] = (t, name, tuple(arr)); return True
        if fid is not None and name == fid:
            b = _base(t)                       # coerce to the token's stored type
            if b in (0x10, 0x11, 0x12): value = int(value)
            elif b == 0x02: value = float(value)
            parsed['tokens'][i] = (t, name, value); return True
    return False

# ---- structured layer view (for editing) ----
def layers(parsed):
    """Group tokens into 10 layer dicts (+ the leading graphic-name field)."""
    toks = parsed['tokens']
    out = []; cur = None
    prefix = []
    for (t,name,v) in toks:
        if t == 0x4d and name == 0x01:        # start of a new layer
            if cur is not None: out.append(cur)
            cur = {}
        if cur is None:
            prefix.append((t,name,v)); continue
        if t == 0x00: continue
        cur[FIELD.get(name, '#%02x'%name)] = v
    if cur is not None: out.append(cur)
    return prefix, out

if __name__ == '__main__':
    p = sys.argv[1] if len(sys.argv)>1 else 'game-playable-us/Save/VioletVandal.GRF'
    parsed = parse(p)
    rebuilt = serialize(parsed)
    orig = parsed['raw']
    print("ROUND-TRIP:", "BYTE-IDENTICAL ✓" if rebuilt == orig else "DIFFERS ✗")
    if rebuilt != orig:
        for i in range(min(len(rebuilt),len(orig))):
            if rebuilt[i]!=orig[i]:
                print("first diff @0x%x: orig=%02x rebuilt=%02x"%(i,orig[i],rebuilt[i])); break
        print("len orig=%d rebuilt=%d"%(len(orig),len(rebuilt)))
