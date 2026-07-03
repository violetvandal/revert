#!/usr/bin/env python3
"""gen_radio.py — install Violet Vandal Radio as a SECOND soundtrack that
coexists with the original (for the in-game MOD_VVR_RADIO toggle).

For each pre-encoded radio .bik (tools/bink/radio/bik/<Track>.bik):
  SongID   = "VVR_<Track>"            (e.g. VVR_Hot_Pursuit)
  path     = "music\\vag\\songs\\<SongID>"   (engine logical path)
  filename = crc32_noxor(SongID) big-endian hex .bik   (the on-disk name)
Copies each into <gamedir>/Data/streams/music under its NEW checksum name
(does NOT touch the original licensed .bik), writes the radio credits file,
and emits the `playlist_tracks_radio` NeverScript block to
tools/bink/radio/playlist_tracks_radio.ns for pasting into skater_sfx.ns.

Pure copy (no wine) -> safe to wire into rebuild-playable.sh.

Usage:  gen_radio.py <gamedir>
"""
import sys, os, csv, zlib, shutil

HERE = os.path.dirname(os.path.abspath(__file__))
BIK_DIR = os.path.join(HERE, "bik")
MANIFEST = os.path.join(HERE, "src", "manifest.tsv")
NS_OUT = os.path.join(HERE, "playlist_tracks_radio.ns")
BAND = "Violet Vandal Radio"

# genre tabs: 0=Punk, 1=Hip Hop, 2=Rock/Other  (so all three jukebox tabs fill)
GENRE1 = {"Funkorama", "Hep_Cats", "Sneaky_Snitch", "Run_Amok", "Space_Jazz"}
GENRE0 = {"Hot_Pursuit", "Killers", "Severe_Tire_Damage",
          "Monkeys_Spinning_Monkeys", "Spazzmatica_Polka"}


def ck(s):
    return (zlib.crc32(s.lower().encode("latin1")) ^ 0xFFFFFFFF) & 0xFFFFFFFF


def load_titles():
    t = {}
    if os.path.exists(MANIFEST):
        with open(MANIFEST, newline="") as f:
            for r in csv.reader(f, delimiter="\t"):
                if len(r) >= 2:
                    t[os.path.splitext(r[0])[0]] = r[1]
    return t


def main():
    if len(sys.argv) < 2:
        print(__doc__); sys.exit(2)
    gamedir = sys.argv[1]
    music = os.path.join(gamedir, "Data", "streams", "music")
    if not os.path.isdir(music):
        sys.exit("no music dir: " + music)
    titles = load_titles()
    biks = sorted(f for f in os.listdir(BIK_DIR) if f.endswith(".bik"))
    if not biks:
        sys.exit("no radio .bik in " + BIK_DIR)

    rows, ns = [], []
    for b in biks:
        base = os.path.splitext(b)[0]              # Hot_Pursuit
        songid = "VVR_" + base                     # VVR_Hot_Pursuit
        path = "music\\vag\\songs\\" + songid
        fn = "%08x.bik" % ck(songid)
        genre = 0 if base in GENRE0 else 1 if base in GENRE1 else 2
        title = titles.get(base, base.replace("_", " "))
        shutil.copyfile(os.path.join(BIK_DIR, b), os.path.join(music, fn))
        rows.append((title, genre, fn))
        # NS: backslashes doubled (ns string escaping)
        ns.append('    {band="%s" track_title="%s" genre=%d path="%s"} '
                  % (BAND, title.replace('"', "'"), genre, path.replace("\\", "\\\\")))

    print("copied %d radio .bik into %s (new checksum names)" % (len(rows), music))

    # credits (CC-BY)
    cred = ["Violet Vandal Radio - royalty-free soundtrack", "=" * 44, "",
            "Tracks (selectable via MOD OPTIONS -> Soundtrack):", ""]
    for title, _, _ in sorted(rows):
        cred.append("  %s" % title)
    cred += ["", "Music by Kevin MacLeod (incompetech.com),",
             "licensed under Creative Commons: By Attribution 4.0 International",
             "(https://creativecommons.org/licenses/by/4.0/).", ""]
    with open(os.path.join(music, "VIOLET_VANDAL_RADIO_credits.txt"), "w") as f:
        f.write("\n".join(cred))

    block = "playlist_tracks_radio = [\n" + "\n".join(ns) + "\n] "
    with open(NS_OUT, "w") as f:
        f.write(block + "\n")
    print("wrote NS block -> %s (%d entries)" % (NS_OUT, len(rows)))


if __name__ == "__main__":
    main()
