#!/usr/bin/env python3
"""
Add (or update) a non-Steam shortcut in Steam's binary shortcuts.vdf — dependency-free.

Used by `revert setup` on the Steam Deck to register "THUG2: Violet Vandal Edition"
-> play-qol.sh with zero clicks. shortcuts.vdf is Valve's binary KeyValues format:
  0x00 key\0 <children> 0x08   nested object
  0x01 key\0 value\0           string
  0x02 key\0 <4 bytes LE>      int32
  0x08                         end-of-object
The file is one implicit root object; shortcuts.vdf nests root -> "shortcuts" -> "0","1",...

SAFETY: round-trips the existing file byte-identical before writing; backs up to
shortcuts.vdf.bak; refuses to run while Steam is running (Steam would overwrite us).

  add-steam-shortcut.py --name NAME --exe PATH [--startdir DIR] [--icon PATH]
                        [--art DIR] [--vdf FILE] [--selftest-only] [--remove]

  --art DIR installs library artwork: DIR/{cover,header,hero,logo,icon}.png are copied
  into Steam's userdata .../config/grid/ as <appid>{p,,_hero,_logo,_icon}.png so the
  shortcut shows a real cover/hero/logo instead of a blank tile.
"""
import sys, os, zlib, struct, glob, argparse, subprocess


# ---- binary VDF parse / serialize (generic) ----------------------------------
def parse(b):
    pos = 0
    def cstr():
        nonlocal pos
        e = b.index(0, pos); s = b[pos:e]; pos = e + 1; return s
    def obj():
        nonlocal pos
        items = []
        while True:
            t = b[pos]; pos += 1
            if t == 0x08:
                return items
            k = cstr()
            if t == 0x00:   v = obj()
            elif t == 0x01: v = cstr()
            elif t == 0x02: v = b[pos:pos+4]; pos += 4
            else: raise ValueError(f"bad type 0x{t:02x} at {pos-1}")
            items.append([t, k, v])
    return obj()  # consumes the root terminator 0x08


def ser(items):
    out = bytearray()
    for t, k, v in items:
        out.append(t); out += k + b"\x00"
        if t == 0x00:   out += ser(v)        # children + this object's 0x08
        elif t == 0x01: out += v + b"\x00"
        elif t == 0x02: out += v
    out.append(0x08)                          # close this object
    return bytes(out)


# ---- helpers -----------------------------------------------------------------
def get(entry, key):
    for it in entry:
        if it[1] == key.encode():
            return it
    return None

def i32(n): return struct.pack("<i", n)

def shortcut_appid(exe_q, name):
    """The unsigned 32-bit appid Steam stores for a shortcut AND uses for the grid
    artwork filenames (<appid>p.png cover, <appid>_hero.png, <appid>_logo.png, …)."""
    return (zlib.crc32(exe_q.encode() + name.encode()) | 0x80000000) & 0xFFFFFFFF


# grid artwork: source filename in the --art dir -> suffix on the appid-keyed grid file
ART_MAP = {
    "cover.png":  "p.png",      # 600x900 portrait capsule — the library tile/cover
    "header.png": ".png",       # 460x215 landscape capsule
    "hero.png":   "_hero.png",  # 3840x1240 banner behind the Play button
    "logo.png":   "_logo.png",  # transparent logo over the hero
    "icon.png":   "_icon.png",
}


def install_art(vdf, appid, art_dir):
    import shutil
    grid = os.path.join(os.path.dirname(vdf), "grid")
    os.makedirs(grid, exist_ok=True)
    done = []
    for src, suf in ART_MAP.items():
        p = os.path.join(art_dir, src)
        if os.path.isfile(p):
            shutil.copyfile(p, os.path.join(grid, f"{appid}{suf}"))
            done.append(f"{src}->{appid}{suf}")
    return grid, done


def make_entry(index, name, exe_q, startdir_q, icon):
    appid = shortcut_appid(exe_q, name)
    return [
        [0x02, b"appid", struct.pack("<I", appid)],
        [0x01, b"AppName", name.encode()],
        [0x01, b"Exe", exe_q.encode()],
        [0x01, b"StartDir", startdir_q.encode()],
        [0x01, b"icon", icon.encode()],
        [0x01, b"ShortcutPath", b""],
        [0x01, b"LaunchOptions", b""],
        [0x02, b"IsHidden", i32(0)],
        [0x02, b"AllowDesktopConfig", i32(1)],
        [0x02, b"AllowOverlay", i32(1)],
        [0x02, b"OpenVR", i32(0)],
        [0x02, b"Devkit", i32(0)],
        [0x01, b"DevkitGameID", b""],
        [0x02, b"DevkitOverrideAppID", i32(0)],
        [0x02, b"LastPlayTime", i32(0)],
        [0x01, b"FlatpakAppID", b""],
        [0x01, b"sortas", b""],
        [0x00, b"tags", []],
    ]


def find_vdf():
    for pat in ("~/.local/share/Steam/userdata/*/config/shortcuts.vdf",
                "~/.steam/steam/userdata/*/config/shortcuts.vdf"):
        hits = glob.glob(os.path.expanduser(pat))
        if hits:
            return hits[0]
    return None


def steam_running():
    try:
        return subprocess.run(["pgrep", "-x", "steam"], capture_output=True).returncode == 0
    except Exception:
        return False


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--name"); ap.add_argument("--exe")
    ap.add_argument("--startdir", default=""); ap.add_argument("--icon", default="")
    ap.add_argument("--vdf", default=None)
    ap.add_argument("--selftest-only", action="store_true")
    ap.add_argument("--remove", action="store_true", help="remove the shortcut named --name")
    ap.add_argument("--art", default=None,
                    help="dir with cover/header/hero/logo/icon .png -> install into grid/ keyed to the appid")
    a = ap.parse_args()

    vdf = a.vdf or find_vdf()
    if not vdf or not os.path.exists(vdf):
        print("shortcuts.vdf not found (open Steam once to create it)", file=sys.stderr); return 2
    raw = open(vdf, "rb").read()

    # SELF-TEST: parse -> serialize must reproduce the file exactly, or we never write.
    root = parse(raw)
    if ser(root) != raw:
        print("REFUSING: binary-VDF round-trip mismatch — parser does not match this file", file=sys.stderr)
        return 3
    print(f"round-trip OK ({len(raw)} bytes) {vdf}")
    if a.selftest_only:
        return 0

    if not a.name or (not a.exe and not a.remove):
        print("need --name and --exe (or --name with --remove)", file=sys.stderr); return 2
    if steam_running():
        print("REFUSING: Steam is running — close Steam first (it would overwrite the file)", file=sys.stderr)
        return 4

    shortcuts = next((it for it in root if it[1] == b"shortcuts"), None)
    if shortcuts is None:
        print("no 'shortcuts' object", file=sys.stderr); return 3
    entries = shortcuts[2]

    if a.remove:
        kept = [e for e in entries if not ((g := get(e[2], "AppName")) and g[2] == a.name.encode())]
        if len(kept) == len(entries):
            print(f"no shortcut named '{a.name}' — nothing to remove"); return 0
        for i, e in enumerate(kept):          # renumber the index keys 0,1,2,…
            e[1] = str(i).encode()
        shortcuts[2] = kept
        out = ser(root); parse(out)
        open(vdf + ".bak", "wb").write(raw); open(vdf, "wb").write(out)
        print(f"removed '{a.name}'  ({len(entries)} -> {len(kept)} entries; backup .bak)")
        return 0

    exe_q = a.exe if a.exe.startswith('"') else f'"{a.exe}"'
    sd = a.startdir or os.path.dirname(a.exe)
    sd_q = sd if sd.startswith('"') else f'"{sd}"'

    # update existing by AppName, else append
    target = None
    for _, _key, ent in entries:
        n = get(ent, "AppName")
        if n and n[2] == a.name.encode():
            target = ent; break
    if target:
        get(target, "Exe")[2] = exe_q.encode()
        get(target, "StartDir")[2] = sd_q.encode()
        if a.icon: get(target, "icon")[2] = a.icon.encode()
        action = "updated"
    else:
        idx = str(len(entries))
        entries.append([0x00, idx.encode(), make_entry(idx, a.name, exe_q, sd_q, a.icon)])
        action = "added"

    out = ser(root)
    parse(out)  # sanity: it must re-parse
    open(vdf + ".bak", "wb").write(raw)
    open(vdf, "wb").write(out)
    print(f"{action} '{a.name}' -> {exe_q}  ({len(raw)} -> {len(out)} bytes; backup .bak)")

    if a.art:
        if os.path.isdir(a.art):
            appid = shortcut_appid(exe_q, a.name)
            grid, done = install_art(vdf, appid, a.art)
            print(f"installed {len(done)} artwork file(s) -> {grid}"
                  + (f"  [{', '.join(done)}]" if done else " (none found)"))
        else:
            print(f"--art dir not found: {a.art}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
