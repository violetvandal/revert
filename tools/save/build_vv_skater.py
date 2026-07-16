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

NS = os.path.join(ROOT, 'tools', 'neverscript', 'ns')
SRC = os.path.join(ROOT, 'mods', 'src', 'vv-skater', 'source')
# (pristine .qb, mod .ns output, the vv_profile injector to apply)
TARGETS = [
    (os.path.join(ROOT, 'game-pristine-us', 'Data', 'scripts', 'game', 'skater', 'skater_profile.qb'),
     os.path.join(SRC, 'skater_profile.ns'), 'inject', '+Violet Vandal roster entry'),
    (os.path.join(ROOT, 'game-pristine-us', 'Data', 'scripts', 'game', 'menu', 'sprites.qb'),
     os.path.join(SRC, 'sprites.ns'), 'inject_sprites', '+ss_vv portrait in the load array'),
]


def build(pristine, out, injector, note):
    if not os.path.exists(pristine):
        raise SystemExit('pristine qb not found: %s' % pristine)
    tmp = '/tmp/_vv_%s.ns' % os.path.basename(out).split('.')[0]
    if os.path.exists(tmp):
        os.remove(tmp)
    subprocess.run([NS, '-d', pristine, '-o', tmp], capture_output=True, check=True)
    lines = open(tmp).read().split('\n')
    result = getattr(vv_profile, injector)(lines)
    os.makedirs(os.path.dirname(out), exist_ok=True)
    open(out, 'w').write('\n'.join(result))
    print('wrote %s (%s, %d lines)' % (out, note, len(result)))


def main():
    for pristine, out, injector, note in TARGETS:
        build(pristine, out, injector, note)


if __name__ == '__main__':
    main()
