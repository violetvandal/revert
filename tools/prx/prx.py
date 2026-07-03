#!/usr/bin/env python3
# THUG2 .prx (PRE archive) unpacker/repacker.
# Format: header[12] = <u32 totalSize><u32 version=0xABCD0003><u32 numFiles>
#   then per file: <u32 dataSize><u32 compSize><u32 nameLen><u32 nameCRC>
#                  <name (nameLen bytes; null-terminated + 4-byte aligned)>
#                  <blob (compSize bytes if compSize else dataSize; 4-byte aligned)>
# compSize>0 => blob is LZSS-compressed to dataSize; compSize==0 => stored raw.
# The game's loader accepts raw entries, so we can replace a file with an
# uncompressed blob (csize=0) without needing the compressor.
import struct, sys

def align4(x): return (x + 3) & ~3

def parse(d):
    total, ver, nfiles = struct.unpack_from('<III', d, 0)
    assert ver == 0xABCD0003, "bad version 0x%x" % ver
    entries = []
    off = 12
    for _ in range(nfiles):
        dsize, csize, nlen, crc = struct.unpack_from('<IIII', d, off)
        # nlen is the on-disk field: already null-terminated and 4-byte aligned,
        # so the name occupies exactly nlen bytes.
        name = d[off+16:off+16+nlen].rstrip(b'\0')
        dstart = off + 16 + nlen
        blen = csize if csize else dsize
        # capture the blob INCLUDING its alignment padding so round-trips are
        # byte-identical (original padding is not always zero-filled).
        blob = d[dstart:dstart+align4(blen)]
        entries.append({'dsize': dsize, 'csize': csize, 'nlen': nlen, 'crc': crc,
                        'name': name, 'blob': blob})
        off = dstart + align4(blen)
    assert off == len(d), "trailing data: ended 0x%x of 0x%x" % (off, len(d))
    return ver, entries

def build(ver, entries):
    body = bytearray()
    for e in entries:
        nlen = e['nlen']
        name_field = e['name'] + b'\0' * (nlen - len(e['name']))
        blob = e['blob']
        # blob already carries its 4-byte alignment padding (see parse / replace)
        assert len(blob) % 4 == 0, "blob not aligned for %r" % e['name']
        body += struct.pack('<IIII', e['dsize'], e['csize'], nlen, e['crc'])
        body += name_field
        body += blob
    total = 12 + len(body)
    return struct.pack('<III', total, ver, len(entries)) + bytes(body)

def find(entries, name):
    nl = name.lower().replace('/', '\\')
    for e in entries:
        # some original name fields carry junk after a NUL terminator (e.g.
        # b'...global_flags.qb\x00ca'); match on the real name before the NUL.
        stored = e['name'].split(b'\0', 1)[0].decode('latin1').lower().replace('/', '\\')
        if stored == nl:
            return e
    return None

if __name__ == '__main__':
    cmd = sys.argv[1]
    if cmd == 'roundtrip':
        d = open(sys.argv[2], 'rb').read()
        ver, entries = parse(d)
        print("files=%d  identical=%s" % (len(entries), build(ver, entries) == d))
    elif cmd == 'list':
        ver, entries = parse(open(sys.argv[2], 'rb').read())
        for e in entries:
            print("%-9d comp=%-9d %s" % (e['dsize'], e['csize'], e['name'].decode('latin1')))
    elif cmd == 'extract':           # prx.py extract <prx> <name-in-archive> <out>
        ver, entries = parse(open(sys.argv[2], 'rb').read())
        e = find(entries, sys.argv[3])
        assert e, "not found: " + sys.argv[3]
        assert e['csize'] == 0, "entry is compressed (csize=%d); decompressor not implemented" % e['csize']
        open(sys.argv[4], 'wb').write(e['blob'][:e['dsize']])
        print("extracted %s (%d bytes, raw)" % (sys.argv[3], e['dsize']))
    elif cmd == 'replace':           # prx.py replace <prx> <name-in-archive> <newfile> <out>
        ver, entries = parse(open(sys.argv[2], 'rb').read())
        e = find(entries, sys.argv[3])
        assert e, "not found: " + sys.argv[3]
        newdata = open(sys.argv[4], 'rb').read()
        e['dsize'] = len(newdata); e['csize'] = 0                       # store uncompressed
        e['blob'] = newdata + b'\0' * (align4(len(newdata)) - len(newdata))  # pad to 4
        open(sys.argv[5], 'wb').write(build(ver, entries))
        print("replaced %s with %d bytes (uncompressed); wrote %s" % (sys.argv[3], len(newdata), sys.argv[5]))
    elif cmd == 'replacez':          # prx.py replacez <prx> <name> <newfile> <out>  (LZSS-compressed)
        import lzss
        ver, entries = parse(open(sys.argv[2], 'rb').read())
        e = find(entries, sys.argv[3])
        assert e, "not found: " + sys.argv[3]
        newdata = open(sys.argv[4], 'rb').read()
        comp = lzss.compress(newdata)
        # safety: the entry is only valid if the game's decoder reproduces it
        assert lzss.decompress(comp, len(newdata)) == newdata, "compress round-trip mismatch!"
        e['dsize'] = len(newdata); e['csize'] = len(comp)
        e['blob'] = comp + b'\0' * (align4(len(comp)) - len(comp))
        open(sys.argv[5], 'wb').write(build(ver, entries))
        print("replaced %s: %d -> %d bytes (compressed %.0f%%); wrote %s" % (
            sys.argv[3], len(newdata), len(comp), 100*len(comp)/len(newdata), sys.argv[5]))
