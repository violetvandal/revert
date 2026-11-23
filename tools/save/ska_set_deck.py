#!/usr/bin/env python3
# ska_set_deck.py — swap the created skater's deck graphic in a THUG2 .SKA save and
# re-sign the file, e.g. to rescue a save whose deck is a decks-pack (VV) graphic.
#
#   ska_set_deck.py <in.SKA> <out.SKA> [new desc_id]      (default: "BH Deck 1")
#   ska_set_deck.py <in.SKA> --show
#
# Why: PreCutscene unloads skaterparts.pre, and the VV HD deck textures live only there,
# so a cutscene that rebuilds the skater looks the texture up, gets NULL and crashes at
# THUG2.exe 0x4C86E3 (read of 0x10). Stock decks survive; VV decks don't.
#
# Save layout (version 0x25): header <u32 crc><u32 summary_crc><u32 summary_size>
# <u32 data_size><u32 version>, summary at 0x14. Both CRCs are Neversoft CRC32
# (zlib.crc32 ^ 0xFFFFFFFF):
#   crc         = over bytes [0, data_size) with the crc field itself zeroed
#   summary_crc = over bytes [0x14, 0x14 + summary_size)
# Deck selection record: 8a bb 8d 1e <desc checksum:4> (slot 0xbb = deck_graphic).
import struct, sys, zlib


def ck(s):
    return (zlib.crc32(s.lower().encode('latin1')) ^ 0xFFFFFFFF) & 0xFFFFFFFF


def ns_crc(b):
    return (zlib.crc32(bytes(b)) ^ 0xFFFFFFFF) & 0xFFFFFFFF


def verify(d):
    crc, scrc, ssize, dsize, ver = struct.unpack_from('<5I', d, 0)
    b = bytearray(d[:dsize]); b[0:4] = b'\0\0\0\0'
    return ns_crc(b) == crc and ns_crc(d[0x14:0x14 + ssize]) == scrc


def resign(d):
    crc, scrc, ssize, dsize, ver = struct.unpack_from('<5I', d, 0)
    struct.pack_into('<I', d, 4, ns_crc(d[0x14:0x14 + ssize]))
    b = bytearray(d[:dsize]); b[0:4] = b'\0\0\0\0'
    struct.pack_into('<I', d, 0, ns_crc(b))


def deck_records(d):
    ap = d.find(struct.pack('<I', ck('appearance')))
    if ap < 0:
        raise SystemExit('no appearance block')
    out, i = [], ap
    while True:
        i = d.find(b'\x8a\xbb\x8d\x1e', i, ap + 0x900)
        if i < 0:
            return out
        out.append(i + 4); i += 4


def main():
    a = sys.argv[1:]
    if not a:
        raise SystemExit(__doc__ if __doc__ else 'usage: ska_set_deck.py <in> <out> [desc_id]')
    d = bytearray(open(a[0], 'rb').read())
    if not verify(d):
        raise SystemExit('input checksums do not verify — not a v0x25 save, or already damaged')
    recs = deck_records(d)
    for off in recs:
        print('deck_graphic at 0x%x = %08x' % (off, struct.unpack_from('<I', d, off)[0]))
    if len(a) > 1 and a[1] == '--show':
        return
    # The first record after `appearance` is the created skater's own deck; later ones
    # belong to other profile blocks (stock decks) and are left alone.
    if not recs:
        raise SystemExit('no deck_graphic record found')
    new = a[2] if len(a) > 2 else 'BH Deck 1'
    struct.pack_into('<I', d, recs[0], ck(new))
    resign(d)
    assert verify(d)
    open(a[1], 'wb').write(d)
    print('set deck_graphic -> %r (%08x), re-signed, wrote %s' % (new, ck(new), a[1]))


if __name__ == '__main__':
    main()
