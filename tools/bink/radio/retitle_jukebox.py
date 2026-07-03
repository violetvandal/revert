#!/usr/bin/env python3
"""retitle_jukebox.py — rewrite the THUG2 jukebox track names to match the
Violet Vandal Radio tracks that actually play in each slot.

The playlist (band/track_title/genre/path) is defined as `playlist_tracks` in
skater_sfx.ns (a mod-options-menu source). Each entry's .bik filename =
crc32_noxor(lowercased last path component), big-endian hex. apply_radio.py
writes assignment.json mapping slot-hex -> {title,artist}. This rewrites each
entry's band/track_title from that map (band -> "Violet Vandal Radio",
track_title -> the track), leaving genre + path untouched so the file mapping
holds. Idempotent. Recompile+inject afterwards (mod-options-menu mod).

Usage:  retitle_jukebox.py [--revert]
"""
import re, os, json, zlib, sys

HERE = os.path.dirname(os.path.abspath(__file__))
NS = os.path.normpath(os.path.join(HERE, "..", "..", "..",
     "mods/src/mod-options-menu/source/skater_sfx.ns"))
ASSIGN = os.path.join(HERE, "assignment.json")
BAND = "Violet Vandal Radio"
ENTRY = re.compile(
    r'\{band="(?P<band>(?:[^"\\]|\\.)*)" track_title="(?P<title>(?:[^"\\]|\\.)*)"'
    r' genre=(?P<genre>\d+) path="(?P<path>(?:[^"\\]|\\.)*)"\}')


def ck(s):
    return (zlib.crc32(s.lower().encode("latin1")) ^ 0xFFFFFFFF) & 0xFFFFFFFF


def main():
    revert = "--revert" in sys.argv
    src = open(NS).read()
    assign = json.load(open(ASSIGN)) if os.path.exists(ASSIGN) else {}
    if not assign and not revert:
        sys.exit("missing %s (run apply_radio.py first)" % ASSIGN)

    n = [0]
    def repl(mo):
        d = mo.groupdict()
        songid = d["path"].replace("\\\\", "\\").split("\\")[-1]
        h = "%08x" % ck(songid)
        track = assign.get(h)
        if not track:
            return mo.group(0)
        n[0] += 1
        title = track["title"].replace('"', "'")
        return ('{band="%s" track_title="%s" genre=%s path="%s"}'
                % (BAND, title, d["genre"], d["path"]))

    out = ENTRY.sub(repl, src)
    if out == src:
        print("no changes (already retitled, or no matches)")
        return
    open(NS, "w").write(out)
    print("retitled %d jukebox entries in %s" % (n[0], os.path.relpath(NS)))
    print("-> recompile+inject: thugkit/rebuild applies mod-options-menu (skater_sfx.qb)")


if __name__ == "__main__":
    main()
