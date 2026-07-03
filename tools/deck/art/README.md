# Steam library artwork (Deck)

`revert setup` installs these into Steam's `userdata/<id>/config/grid/` keyed to the
shortcut's appid, so the library entry shows a real cover/hero/logo instead of a blank
tile. Drop PNGs here with these exact names:

| File         | Becomes `grid/<appid>…` | Role / recommended size            |
|--------------|-------------------------|------------------------------------|
| `cover.png`  | `<appid>p.png`          | Portrait library tile — 600×900    |
| `header.png` | `<appid>.png`           | Landscape capsule — 460×215        |
| `hero.png`   | `<appid>_hero.png`      | Banner behind Play — 3840×1240     |
| `logo.png`   | `<appid>_logo.png`      | Transparent logo over hero — 16:9  |
| `icon.png`   | `<appid>_icon.png`      | Small icon — 256×256               |

Any missing file is simply skipped. After install, Steam must restart to pick them up —
setup already does the close/reopen, so it's automatic.

Source: polished community art (e.g. SteamGridDB) with a small "VV Edition" mark composited
on. Regenerate the marked versions from base images with `make_art.py`.
