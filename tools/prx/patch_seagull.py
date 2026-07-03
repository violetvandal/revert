#!/usr/bin/env python3
# Binary-patch the REAL THUG2 AU_sfx.qb bytecode to silence the seagull fly-up
# cry: set vol=150 -> vol=0 on the two Obj_PlayStream AU_Seagull_Fly_Up_01/02
# calls. Same-size, structure-preserving (no NeverScript recompile).
import struct, sys
data = bytearray(open(sys.argv[1], 'rb').read())

# 1. Build hash->name from checksum-entry records (0x2b <u32 hash> <name\0>).
names = {}
i = 0
while i < len(data):
    if data[i] == 0x2b and i + 5 < len(data):
        h = struct.unpack_from('<I', data, i+1)[0]
        j = i + 5
        while j < len(data) and data[j] != 0:
            j += 1
        nm = data[i+5:j]
        if nm and all(32 <= c < 127 for c in nm):
            names[h] = nm.decode('latin1')
            i = j + 1
            continue
    i += 1

want = {n.lower(): h for h, n in names.items()
        if n.lower() in ('au_seagull_fly_up_01', 'au_seagull_fly_up_02')}
print("seagull sound hashes:", {k: hex(v) for k, v in want.items()})
assert len(want) == 2, "did not find both seagull sound symbols"

VOL150 = bytes([0x17, 0x96, 0x00, 0x00, 0x00])  # int opcode + 150
VOL0   = bytes([0x17, 0x00, 0x00, 0x00, 0x00])  # int opcode + 0
patches = 0
for nm, h in want.items():
    sig = bytes([0x16]) + struct.pack('<I', h)   # checksum opcode + hash (the Obj_PlayStream arg)
    pos = data.find(sig)
    assert pos != -1, "sound hash not found in code: " + nm
    # find vol=150 within this call (before the next checksum-with-a-name boundary)
    region = data.find(VOL150, pos, pos + 64)
    assert region != -1, "vol=150 not found near " + nm
    data[region:region+5] = VOL0
    patches += 1
    print("patched %s: vol=150 -> vol=0 at 0x%x" % (nm, region))

assert patches == 2
open(sys.argv[2], 'wb').write(data)
print("wrote", sys.argv[2], len(data), "bytes")
