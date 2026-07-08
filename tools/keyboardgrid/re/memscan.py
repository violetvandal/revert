#!/usr/bin/env python3
# Scan a live process's memory (/proc/<pid>/mem) for a string (ASCII + UTF-16LE + upper/lower).
# Usage: memscan.py <pid> <needle>
import sys, re

def regions(pid):
    out = []
    for line in open(f"/proc/{pid}/maps"):
        m = re.match(r"([0-9a-f]+)-([0-9a-f]+) (\S+)", line)
        if not m: continue
        lo, hi, perms = int(m.group(1), 16), int(m.group(2), 16), m.group(3)
        if 'r' not in perms: continue
        # skip file-backed device/lib regions? keep anonymous + heap (where a game name buffer lives)
        out.append((lo, hi, perms, line.rstrip()))
    return out

def main():
    pid = int(sys.argv[1]); needle = sys.argv[2]
    pats = {
        "ascii": needle.encode('latin1'),
        "ascii_up": needle.upper().encode('latin1'),
        "utf16": needle.encode('utf-16-le'),
        "utf16_up": needle.upper().encode('utf-16-le'),
    }
    mem = open(f"/proc/{pid}/mem", "rb", 0)
    hits = 0
    for lo, hi, perms, raw in regions(pid):
        size = hi - lo
        if size > (256 << 20): continue
        try:
            mem.seek(lo); buf = mem.read(size)
        except (OSError, ValueError, OverflowError):
            continue
        for name, pat in pats.items():
            start = 0
            while True:
                i = buf.find(pat, start)
                if i < 0: break
                va = lo + i
                # show 32 bytes of context
                ctx = buf[max(0,i-4):i+len(pat)+12]
                print(f"[{name}] va=0x{va:x} region={perms} {raw.split()[-1] if len(raw.split())>5 else ''}")
                print(f"        ctx: {ctx.hex(' ')}")
                hits += 1; start = i + 1
    print(f"--- {hits} hits for {needle!r} ---")

if __name__ == "__main__":
    main()
