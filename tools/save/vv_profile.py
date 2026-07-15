#!/usr/bin/env python3
# vv_profile.py — make Violet Vandal a selectable playable character.
#
# THUG2's playable roster (the Select Skater screen: pros + the Custom slot) is
# `master_skater_list` in scripts/game/skater/skater_profile.qb — NOT the pre-made
# create-a-skater list (custom_male_appearances, which only the debug menu reads). This
# injects one more roster entry, "Violet Vandal", built from her real created-skater
# appearance. See [[project_violet_vandal_playable]], [[project_save_format]].
#
# Self-contained: both the appearance struct AND the roster entry go into skater_profile.qb,
# so there's no cross-file dependency on cas_skater_m.qb and no conflict with playas-pro.
#
# She is shown + selectable because her entry carries neither `not_in_frontend` (which
# ForEachSkaterProfile skips) nor `is_secret` (which greys the icon until an unlock flag).
# is_locked=0 keeps her available from the start; is_male=0 + voice=female1 match her body.
#
# The appearance below is reverse-engineered from the real `Violet Vandal.SKA`
# (tools/save/skater_to_roster.py decodes any save; all-stock CAS parts). Tattoo slots are a
# best-effort read of the save's compiled part enum — cosmetic, verify in-game.

STRUCT_NAME = 'appearance_vv'
DISPLAY_NAME = 'Violet Vandal'

APPEARANCE = '''appearance_vv = {
    body={desc_id=FemaleBody}
    board={desc_id=`default`}
    deck_graphic={desc_id=`BH Deck 1`}
    griptape={desc_id=`Generic 17`}
    skater_f_head={desc_id=Goth h=0 s=0 v=58 use_default_hsv=0}
    skater_f_hair={desc_id=Cropped h=265 s=90 v=60 use_default_hsv=0}
    eyes={desc_id=`Hazel eyes`}
    skater_f_torso={desc_id=`Button Open SS` h=0 s=0 v=60 use_default_hsv=0}
    skater_f_legs={desc_id=`Mini Skirt` h=0 s=0 v=12 use_default_hsv=0}
    skater_f_hands={desc_id=Hands}
    shoes={desc_id=`Boots Tall` h=0 s=0 v=12 use_default_hsv=0}
    accessory1={desc_id=`Spike 2 Band L` h=0 s=0 v=12 use_default_hsv=0}
    accessory2={desc_id=`Silver Watch R`}
    accessory3={desc_id=`Angel Wings` h=0 s=0 v=12 use_default_hsv=0}
    left_bicep_tattoo={desc_id=`Tattoo 4`}
    right_bicep_tattoo={desc_id=`Tattoo 31`}
    left_forearm_tattoo={desc_id=`Tattoo 44`}
    body_shape=female_scale_info
} '''

# master_skater_list entry. Stats are strong-but-fair (namesake character). select_icon
# reuses ss_custom (the Custom Skater portrait) since she has no bespoke icon texture yet.
ENTRY = '''    {
        display_name="Violet Vandal"
        select_icon=ss_custom
        first_name="Violet"
        last_name="Vandal"
        file_name="Unimplemented"
        default_appearance=appearance_vv
        name=violetvandal
        stance=regular
        pushstyle=never_mongo
        trickstyle=street
        has_custom_tag=0
        tag_texture="tags\\\\cas_01"
        sticker_texture="CAGR/Corporate/corp_1"
        skater_family=family_custom
        is_pro=0
        is_male=0
        is_head_locked=0
        is_locked=0
        age=21
        hometown="Berlin"
        points_available=0
        air=11
        run=11
        ollie=11
        speed=11
        spin=11
        `switch`=11
        flip_speed=11
        rail_balance=11
        lip_balance=11
        manual_balance=11
        sponsors=[]
        trick_mapping={}
        default_trick_mapping=CustomTricks
        max_specials=4
        specials={
            [
                {trickslot=SpAir_R_D_Circle trickname=Trick_McTwist}
                {trickslot=SpAir_U_R_Square trickname=Trick_KickFlipUnderFlip}
                {trickslot=SpGrind_R_D_Triangle trickname=Trick_tailblockslide}
                {trickslot=SpMan_D_U_Triangle trickname=Trick_OneFootOneWheel}
                {trickslot=Unassigned trickname=Unassigned}
                {trickslot=Unassigned trickname=Unassigned}
                {trickslot=Unassigned trickname=Unassigned}
                {trickslot=Unassigned trickname=Unassigned}
                {trickslot=Unassigned trickname=Unassigned}
                {trickslot=Unassigned trickname=Unassigned}
                {trickslot=Unassigned trickname=Unassigned}
                {trickslot=Unassigned trickname=Unassigned}
            ]
        }
        voice=female1
    } '''


def inject(lines):
    """Given skater_profile.ns as a list of lines, return a new list with Violet Vandal added:
    her appearance struct before master_skater_list, her roster entry right AFTER the first
    entry (the player's Custom Skater slot) so she sits between it and Tony Hawk. Idempotent."""
    if any(STRUCT_NAME + ' = {' in l for l in lines):
        return lines
    out = list(lines)
    li = next(i for i, l in enumerate(out) if l.startswith('master_skater_list = ['))
    # end of the first roster entry = the first entry-level closer (a '}' indented 4 spaces;
    # deeper braces like specials={...} are indented 8+). Insert her entry just after it.
    def is_entry_closer(s):
        return s.strip() == '}' and len(s) - len(s.lstrip(' ')) == 4
    first_close = next(i for i in range(li + 1, len(out)) if is_entry_closer(out[i]))
    out.insert(first_close + 1, ENTRY)   # after Custom Skater, before Tony Hawk
    out[li:li] = APPEARANCE.split('\n')   # struct definition just above the list
    return out


if __name__ == '__main__':
    import sys
    src = open(sys.argv[1]).read().split('\n') if len(sys.argv) > 1 else []
    sys.stdout.write('\n'.join(inject(src)))
