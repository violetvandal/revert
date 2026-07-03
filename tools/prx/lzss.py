#!/usr/bin/env python3
# THUG2 .prx LZSS decompressor (Neversoft / Okumura-style).
# Format: control byte, LSB-first; bit=1 => literal byte; bit=0 => match.
# Match = 2 bytes (lo, hi): pos = lo | ((hi & 0xF0) << 4); len = (hi & 0x0F) + 3;
#         copy `len` bytes from the 4096-byte ring buffer at `pos`.
# Ring buffer N=4096, write pointer starts at N-F (F=18); init value is irrelevant
# for the files we've seen. Validated: decompresses AU_sfx.qb to exactly dataSize
# and the result decompiles cleanly with NeverScript.
#
# NOTE: only the DEcompressor exists. No compressor yet (we store modified files
# uncompressed in the .prx, which the loader accepts). See project_prx_loading memory.
import struct, sys

def decompress(comp, outsize, N=4096, F=18, THRESHOLD=2, init=0):
    ring = bytearray([init]) * N
    r = N - F
    out = bytearray()
    i = 0
    while len(out) < outsize and i < len(comp):
        flags = comp[i]; i += 1
        for b in range(8):
            if len(out) >= outsize or i >= len(comp):
                break
            if (flags >> b) & 1:                 # literal (LSB-first)
                c = comp[i]; i += 1
                out.append(c); ring[r] = c; r = (r + 1) % N
            else:                                # match
                lo = comp[i]; hi = comp[i + 1]; i += 2
                pos = lo | ((hi & 0xF0) << 4)
                length = (hi & 0x0F) + THRESHOLD + 1
                for k in range(length):
                    if len(out) >= outsize:
                        break
                    c = ring[(pos + k) % N]
                    out.append(c); ring[r] = c; r = (r + 1) % N
    return bytes(out)


def compress(data, N=4096, F=18, THRESHOLD=2, init=0):
    """Encode `data` into a stream the decompress() above reproduces exactly.

    LZSS (Okumura-style) matched to this decoder: control byte LSB-first,
    bit=1 => literal byte, bit=0 => match (lo,hi) where
    pos = lo | ((hi & 0xF0) << 4); len = (hi & 0x0F) + THRESHOLD + 1.
    The decoder copies from a 4096-byte ring whose write pointer starts at
    N-F; pos is the ring offset (r - distance) mod N. We search in input
    coordinates (window = last N bytes, min match 3, max F=18) which is
    equivalent and handles RLE overlap naturally (periodic matches).
    Any valid encoding works; correctness is verified by round-trip.
    """
    MINMATCH = THRESHOLD + 1            # 3
    MAXMATCH = F                        # 18
    n = len(data)
    out = bytearray()
    # hash chain: 3-byte key -> list of positions (most-recent first)
    from collections import defaultdict
    chains = defaultdict(list)

    def key(p):
        return data[p] | (data[p + 1] << 8) | (data[p + 2] << 16)

    p = 0
    r = N - F                          # mirror decoder's ring write pointer
    MAX_CANDIDATES = 256               # cap chain walk for speed
    while p < n:
        flag = 0
        chunk = bytearray()
        for b in range(8):
            if p >= n:
                break
            best_len = 0
            best_dist = 0
            if p + MINMATCH <= n:
                limit = p - N           # oldest allowed source position
                tried = 0
                for src in chains.get(key(p), ()):
                    if src <= limit:
                        break           # chain is newest-first; rest are too old
                    # extend match (input coords; periodic/overlap is valid)
                    l = 0
                    maxl = MAXMATCH if (n - p) >= MAXMATCH else (n - p)
                    while l < maxl and data[src + l] == data[p + l]:
                        l += 1
                    if l > best_len:
                        best_len = l
                        best_dist = p - src
                        if l == MAXMATCH:
                            break
                    tried += 1
                    if tried >= MAX_CANDIDATES:
                        break
            if best_len >= MINMATCH:
                pos = (r - best_dist) % N
                lo = pos & 0xFF
                hi = ((pos >> 4) & 0xF0) | ((best_len - MINMATCH) & 0x0F)
                chunk.append(lo)
                chunk.append(hi)
                # insert hash entries + advance for each covered byte
                for k in range(best_len):
                    if p + 2 < n:
                        chains[key(p)].insert(0, p)
                    p += 1
                    r = (r + 1) % N
            else:
                flag |= (1 << b)        # literal
                if p + 2 < n:
                    chains[key(p)].insert(0, p)
                chunk.append(data[p])
                p += 1
                r = (r + 1) % N
        out.append(flag)
        out += chunk
    return bytes(out)


if __name__ == '__main__':
    # lzss.py extract-from-prx <prx> '<name\with\backslashes>' <out.qb>
    sys.path.insert(0, __file__.rsplit('/', 1)[0])
    import prx
    ver, entries = prx.parse(open(sys.argv[2], 'rb').read())
    e = prx.find(entries, sys.argv[3])
    assert e, "not found: " + sys.argv[3]
    if e['csize'] == 0:
        data = e['blob'][:e['dsize']]
    else:
        data = decompress(e['blob'][:e['csize']], e['dsize'])
    assert len(data) == e['dsize'], "size mismatch %d != %d" % (len(data), e['dsize'])
    open(sys.argv[4], 'wb').write(data)
    print("extracted+decompressed %s -> %s (%d bytes)" % (sys.argv[3], sys.argv[4], len(data)))
