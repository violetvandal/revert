#!/usr/bin/env python3
# build_vv_skater.py — regenerate the vv-skater mod's skater_profile.ns.
#
# Decompiles the pristine skater_profile.qb and injects Violet Vandal as a playable
# roster character (shared tools/save/vv_profile.py: appearance struct + master_skater_list
# entry). The result is committed as the mod's source. Run after changing vv_profile.py or
# the pristine base.
#
#   python3 tools/save/build_vv_skater.py
import os, sys, subprocess
HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, '..', '..'))
sys.path.insert(0, HERE)
import vv_profile

PRISTINE = os.path.join(ROOT, 'game-pristine-us', 'Data', 'scripts', 'game', 'skater', 'skater_profile.qb')
NS = os.path.join(ROOT, 'tools', 'neverscript', 'ns')
OUT = os.path.join(ROOT, 'mods', 'src', 'vv-skater', 'source', 'skater_profile.ns')


def main():
    if not os.path.exists(PRISTINE):
        raise SystemExit('pristine skater_profile.qb not found: %s' % PRISTINE)
    tmp = '/tmp/_vv_sp.ns'
    if os.path.exists(tmp):
        os.remove(tmp)
    subprocess.run([NS, '-d', PRISTINE, '-o', tmp], capture_output=True, check=True)
    lines = open(tmp).read().split('\n')
    out = vv_profile.inject(lines)
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    open(OUT, 'w').write('\n'.join(out))
    print('wrote %s (+Violet Vandal roster entry, %d lines)' % (OUT, len(out)))


if __name__ == '__main__':
    main()
