# In-game credits movies

THUG2-format Bink movies for the edition's credits experience:

- `vvcredits.bik` — the Skatepark Project video + a closing "support / donate" card.
  Reached in-game via **Options → Game Movies → Violet Vandal**.
- `vvabout.bik` — the donation card alone (~20s), for quick access via
  **Game Options → MOD OPTIONS → Support the Skatepark Project**.

`revert build` installs these into `Data/movies/bik/`. The menu entries that play them
live in `mods/src/mainmenu-mod/source/mainmenu_options.ns`
(`movie_file="movies\\vvcredits"` / `"movies\\vvabout"`).

## Regenerating (needs Wine + RAD + an X display)
    tools/bink/credits_card.py tools/bink/work/card.png
    tools/bink/encode_video_bik.sh --card tools/bink/work/card.png --card-secs 8 \
        tools/bink/credits/the-skatepark-project.mp4 tools/bink/credits/vvcredits.bik
    # card-only (vvabout): make a 20s card video, then encode it (see session notes).

Prebuilt .bik are committed so a normal build needs no Wine. Edit the card wording in
`tools/bink/credits_card.py`.
