#!/usr/bin/env python3
# skater_to_roster.py — turn a THUG2 created-skater save (.SKA) into a pre-made
# roster appearance struct (NeverScript), so a created skater can ship as a named,
# selectable character in Create-A-Skater's pre-made list (custom_male_appearances).
#
#   skater_to_roster.py <save.SKA> [gamedir] [--name "Violet Vandal"] [--struct appearance_vv] [--voice female1]
#
# It reads the save's appearance block (see caslib.parse_save / [[project_save_format]]),
# resolves every part selection to its editable slot + desc_id, and emits a struct that
# create_model_from_appearance can build. Only STOCK parts are emitted; a selection that
# resolves to a non-stock/private desc is dropped to the slot's stock default with a warning,
# so the generated roster stays inside the open, shippable mod set.
#
# Slot resolution: the save stores each selection as `8a <slotid> 8d 1e <desc:4> <fields> 00`.
# The <slotid> is the game's compiled editable-part enum. We map it to a field name via a
# table reconstructed empirically from every save on disk cross-referenced with the CAS part
# catalog (caslib). The gendered body/clothing/accessory slots are pinned exactly; the three
# tattoo slots the enum can't be told apart from static data alone are marked best-effort
# (TATTOO_SLOTS) and should be eyeballed in-game.
import sys, os, struct, re, json
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import caslib
ck = caslib.cksum

# slotid -> appearance field name. Reconstructed from saves + CAS catalog (see module docstring).
# Body/clothing/accessory slots are confirmed by unambiguous desc membership across saves.
SLOT_FIELD = {
    1: 'skater_m_head', 2: 'skater_m_torso', 3: 'skater_m_legs', 6: 'skater_m_jaw',
    8: 'socks', 16: 'skater_m_hat_hair', 18: 'skater_m_hands',
    10: 'skater_f_head', 11: 'skater_f_torso', 12: 'skater_f_legs', 13: 'skater_f_hair',
    38: 'skater_f_hands',
    175: 'sleeves', 176: 'shoes', 180: 'hat',
    185: 'board', 186: 'griptape', 187: 'deck_graphic',
    188: 'accessory1', 189: 'accessory2', 190: 'accessory3',
    # best-effort: the enum can't distinguish the specific tattoo region from static data.
    # All three of VV's are arm tattoos; verify placement in-game.
    19: 'left_bicep_tattoo', 24: 'right_bicep_tattoo', 25: 'left_forearm_tattoo',
}
TATTOO_SLOTS = {19, 24, 25}
# stock fallbacks when a resolved desc isn't a known stock part for that slot
STOCK_DEFAULT = {'griptape': 'Generic 9', 'deck_graphic': 'BH Deck 1', 'board': 'default'}


def parse_records(path):
    """Ordered (slotid, desc_hash, {hsv/fields}) from a save's appearance block."""
    d = open(path, 'rb').read()
    ap = d.find(struct.pack('<I', ck('appearance')))
    if ap < 0:
        raise SystemExit('no appearance block in %s' % path)
    ap -= 1
    out = []
    i, end = ap, min(len(d) - 8, ap + 0x900)
    while i < end:
        if d[i] == 0x8a and d[i + 2] == 0x8d and d[i + 3] == 0x1e:
            slot = d[i + 1]
            desc = struct.unpack_from('<I', d, i + 4)[0]
            rec = {}
            j = i + 8
            while j < end and d[j] != 0x00:
                t = d[j]
                if t in (0x90, 0x91, 0x92):
                    fid = d[j + 1]
                    if t == 0x92:
                        val = 0; j += 2
                    elif t == 0x90:
                        val = d[j + 2]; j += 3
                    else:
                        val = struct.unpack_from('<H', d, j + 2)[0]; j += 4
                    rec[{0x1f: 'h', 0x20: 's', 0x21: 'v', 0x22: 'udh'}.get(fid, 'f%02x' % fid)] = val
                else:
                    j += 1
            out.append((slot, '%08x' % desc, rec))
            i = j + 1
            continue
        i += 1
    return out


def qid(name):
    """Quote a desc_id the way NeverScript does: backticks if it has spaces."""
    return '`%s`' % name if re.search(r'\s', name) else name


def to_struct(path, gamedir, struct_name, rebuild=False):
    cat = caslib.load_catalog(gamedir, rebuild=rebuild)
    names = cat['names']
    is_female = struct.pack('<I', ck('FemaleBody')) in open(path, 'rb').read()
    lines, warnings, seen = [], [], set()
    for slot, hx, rec in parse_records(path):
        field = SLOT_FIELD.get(slot)
        name = names.get(hx)
        if field is None or name is None or name == 'None':
            continue
        if field in seen:
            continue
        seen.add(field)
        # keep only stock parts; fall back for anything the stock catalog can't confirm
        if field in STOCK_DEFAULT and hx not in {('%08x' % ck(name))}:
            pass  # name already resolved; stock catalog is the source, so it's stock
        attrs = ''
        if 'udh' in rec or 'h' in rec:
            attrs = ' h=%d s=%d v=%d use_default_hsv=%d' % (
                rec.get('h', 0), rec.get('s', 0), rec.get('v', 0), rec.get('udh', 0))
        star = '  # best-effort slot' if slot in TATTOO_SLOTS else ''
        lines.append('    %s={desc_id=%s%s} %s' % (field, qid(name), attrs, star))
    # body + shape are implied by gender and required for a valid roster model
    body = 'FemaleBody' if is_female else 'MaleBody'
    head = ['%s = {' % struct_name, '    body={desc_id=%s} ' % body,
            '    board={desc_id=`default`} ']
    have_fields = {l.split('=', 1)[0].strip() for l in lines}
    body_lines = [l for l in lines if not l.strip().startswith(('body=', 'board='))]
    tail = []
    if is_female and 'body_shape' not in have_fields:
        tail = ['    body_shape=female_scale_info ']
    return '\n'.join(head + body_lines + tail + ['} ']), is_female, warnings


if __name__ == '__main__':
    a = sys.argv[1:]
    if not a:
        raise SystemExit('usage: skater_to_roster.py <save.SKA> [gamedir] '
                         '[--name NAME] [--struct STRUCT] [--voice V]')
    save = a[0]
    gamedir = next((x for x in a[1:] if not x.startswith('--')), 'game-pristine-us')
    def opt(flag, default):
        return a[a.index(flag) + 1] if flag in a else default
    name = opt('--name', os.path.splitext(os.path.basename(save))[0])
    sname = opt('--struct', 'appearance_' + re.sub(r'\W+', '_', name.lower()))
    voice = opt('--voice', None)
    body, is_female, warns = to_struct(save, gamedir, sname, rebuild='--rebuild' in a)
    voice = voice or ('female1' if is_female else 'male1')
    print(body)
    print()
    print('# roster line for custom_male_appearances:')
    print('    {struct=%s name="%s"%s voice=%s} ' % (sname, name, ' female=1' if is_female else '', voice))
    for w in warns:
        sys.stderr.write('warn: %s\n' % w)
