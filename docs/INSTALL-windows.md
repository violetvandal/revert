# Install on Windows

A beginner-friendly walkthrough for Windows 10 and 11. THUG2 runs **natively** on Windows, so there
is no Wine and no emulation here. You click through a small local app and it does the rest.

> **You bring the game.** Revert is the installer and mods, not the game itself. You need to own
> Tony Hawk's Underground 2 (PC) and have your own copy ready as a folder (the one with a `Data\`
> subfolder) or a `.zip` of it.

## What you need
- Windows 10 or 11 (64-bit).
- Your THUG2 (PC) install folder, or a `.zip` of it.
- A **no-CD `THUG2.exe`** that you supply. Its MD5 must be
  `d464781a2863c833c640f7ff6d377ffe` so the HUD, glyph, and keyboard mods line up. (The original
  disc executable will not run on modern Windows because of its old copy protection.)
- The **DirectX 9** runtime. THUG2 is a 2004 game and Windows 10/11 do not ship these DLLs. The
  setup step installs them for you if you drop Microsoft's DirectX End-User Runtime in place;
  otherwise install it from Microsoft first.

## Step by step

1. **Download the bundle.** Go to `https://github.com/violetvandal/revert/releases/latest` and
   download **`revert-windows-amd64.zip`**.

2. **Unzip it** to a folder you own, for example `C:\Users\<you>\thug2`. (Do not put it under
   `C:\Program Files`; a folder you own avoids permission prompts.)

3. **Add your no-CD executable.** Copy your no-CD `THUG2.exe` into the unzipped folder at
   `game-modded-vanilla\THUG2.exe`.

4. **Run `revert-gui.exe`.** Double-click it. A small management page opens in your browser.

5. **"Windows protected your PC" will appear.** This is expected. Revert is not code-signed, because
   a signing certificate would publish the maintainer's legal name. It is a warning about *who*
   published it, not about *what* it does. Click **More info**, then **Run anyway**.

6. **Click through the panel:** **Setup** (the DirectX step may show one UAC prompt), then
   **Acquire** (point it at your THUG2 folder), then **Build**, then **Play**. Live output streams in
   the page.

That's it. To play again later, just run `revert-gui.exe` and click **Play**, or use the command line
below.

## Controller
Plug in an **Xbox-style pad**. Sticks and face buttons work natively. For the PS2-style shoulder
combos the game cannot bind (get-off-board = LB+RB, rotate = LT/RT), a small helper called
`vv-padbridge.exe` runs automatically while you play and maps them. Just play with the pad connected.

## Command line (optional)
If you prefer a terminal, open one in the unzipped folder:

```
revert.exe doctor
revert.exe setup
revert.exe acquire-game-data --from "C:\path\to\your\THUG2"
revert.exe build
revert.exe run qol
```

Lanes: `run qol` (modded) · `run vanilla` · `run online` (THUG Pro). Options: `--glyphs
xbox|playstation|gamecube|keyboard` and `--soundtrack original|radio`.

## Verifying the download (optional)
Every release publishes a checksum. In PowerShell:

```powershell
Get-FileHash revert-windows-amd64.zip -Algorithm SHA256
```

Compare it against `revert-windows-amd64.zip.sha256` on the same release page. If Windows Defender
ever reports a *named* threat (not the generic unknown-publisher notice), that is a false positive
and we want to hear about it: please open an issue.

## Updating
```
revert.exe update --check     is there a newer release?
revert.exe update             download it, verify, replace, and rebuild
```
Your game data, your built edition, and your `Save\` folder are never touched. Afterwards, close and
reopen `revert-gui.exe`. The GUI has the same two buttons under **Update**.

## THUG Pro (online, optional)
Install THUG Pro (`THUGProSetup.exe`, a native Windows app), then run `revert.exe run online`.

## Cosmetic extras (optional)
The core edition needs no Python. For the optional cosmetic extras (recolors, stickers, licensed
decks), install Python 3 from python.org and run `pip install pillow numpy`; the next `revert.exe
build` picks them up.
