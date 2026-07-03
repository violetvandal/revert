#!/usr/bin/env python3
"""apply_radio.py — install "Violet Vandal Radio" over a game's jukebox.

Streaming Mode v2: replaces every licensed soundtrack .bik in
<gamedir>/Data/streams/music with our pre-encoded royalty-free tracks
(CC0 / CC-BY), cycling the curated set across all stock slots for variety,
and drops a credits file for the CC-BY attribution requirement.

Pre-encoded .bik live in tools/bink/radio/bik/ (made once via encode_bik.sh).
This script does NOT need wine/RAD — it only copies, so it's safe to wire into
rebuild-playable.sh. Idempotent.

Usage:  apply_radio.py <gamedir>            # e.g. game-playable-us
        apply_radio.py <gamedir> --dry-run
"""
import sys, os, shutil, csv

HERE = os.path.dirname(os.path.abspath(__file__))
BIK_DIR = os.path.join(HERE, "bik")
MANIFEST = os.path.join(HERE, "src", "manifest.tsv")
CREDITS_NAME = "VIOLET_VANDAL_RADIO_credits.txt"


def load_manifest():
    rows = {}
    if os.path.exists(MANIFEST):
        with open(MANIFEST, newline="") as f:
            for r in csv.reader(f, delimiter="\t"):
                if len(r) >= 4:
                    rows[os.path.splitext(r[0])[0]] = {"title": r[1], "artist": r[2], "license": r[3]}
    return rows


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    dry = "--dry-run" in sys.argv
    if not args:
        print(__doc__); sys.exit(2)
    gamedir = args[0]
    music = os.path.join(gamedir, "Data", "streams", "music")
    if not os.path.isdir(music):
        sys.exit("no music dir: " + music)

    tracks = sorted(f for f in os.listdir(BIK_DIR) if f.endswith(".bik"))
    if not tracks:
        sys.exit("no pre-encoded .bik in " + BIK_DIR + " (run encode_bik.sh first)")
    slots = sorted(f for f in os.listdir(music)
                   if f.endswith(".bik") and not f.startswith("."))
    man = load_manifest()
    print("radio tracks=%d  jukebox slots=%d  %s"
          % (len(tracks), len(slots), "(dry-run)" if dry else ""))

    used = set()
    assignment = {}   # slot-hex (no .bik) -> {title, artist, license}
    for i, slot in enumerate(slots):
        src_name = tracks[i % len(tracks)]   # round-robin cycle for variety
        used.add(src_name)
        key = os.path.splitext(src_name)[0]
        m = man.get(key, {})
        assignment[os.path.splitext(slot)[0].lower()] = {
            "title": m.get("title", key), "artist": m.get("artist", "?"),
            "license": m.get("license", "?")}
        if not dry:
            shutil.copyfile(os.path.join(BIK_DIR, src_name), os.path.join(music, slot))
    print("%s %d slots from %d unique tracks"
          % ("would write" if dry else "wrote", len(slots), len(used)))

    # sidecar map so the jukebox-retitle step stays in sync with what actually plays
    if not dry:
        import json
        with open(os.path.join(HERE, "assignment.json"), "w") as f:
            json.dump(assignment, f, indent=1, sort_keys=True)
        print("wrote slot->track map -> %s" % os.path.join(HERE, "assignment.json"))

    # credits (CC-BY attribution requirement)
    if not dry:
        lines = ["Violet Vandal Radio - royalty-free soundtrack", "=" * 44, "",
                 "Licensed music used in this build (replacing the original soundtrack):", ""]
        for t in sorted(used):
            key = os.path.splitext(t)[0]
            m = man.get(key, {})
            lines.append("  %s - %s (%s)"
                         % (m.get("artist", "?"), m.get("title", key), m.get("license", "?")))
        lines += ["", "CC BY tracks by Kevin MacLeod are from incompetech.com,",
                  "licensed under Creative Commons: By Attribution 4.0 International",
                  "(https://creativecommons.org/licenses/by/4.0/).", ""]
        with open(os.path.join(music, CREDITS_NAME), "w") as f:
            f.write("\n".join(lines))
        print("wrote credits ->", os.path.join(music, CREDITS_NAME))


if __name__ == "__main__":
    main()
