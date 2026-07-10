THUG2: Violet Vandal Edition — Windows
======================================

This bundle is TOOLING. It ships no game data: you must own Tony Hawk's Underground 2
(PC) and supply your own copy. THUG2 runs natively on Windows, so there is no Wine, no
prefix, and no emulation here.

What you need
-------------
1. Your THUG2 (PC) install folder (the one with a Data\ subfolder), or a .zip of it.
2. A no-CD THUG2.exe (user-supplied). Its md5 must be
   d464781a2863c833c640f7ff6d377ffe so the HUD / glyph / keyboard mods bind correctly.
   Drop it at:  game-modded-vanilla\THUG2.exe
3. Optional: the HQ Xbox audio pack (.7z) at tools\THUG2_HQ_Xbox.7z, and 7-Zip on your
   PATH, for HQ audio.
4. DirectX 9 runtime (d3dx9). THUG2 is a 2004 game and Windows 10/11 do not ship these
   DLLs by default. `revert setup` installs them if you drop Microsoft's DirectX
   End-User Runtime under tools\dx\ (DXSETUP.exe or dxwebsetup.exe); otherwise install it
   yourself from Microsoft.

Quick start (double-click)
--------------------------
Run  revert-gui.exe  — a local web page opens (the management panel). Click through:
Setup -> Acquire (point at your THUG2 folder) -> Build -> Play. Live output streams in
the page. No admin needed (install to a folder you own, e.g. %USERPROFILE%\thug2); only
the DirectX installer may prompt UAC.

"Windows protected your PC" on first run
----------------------------------------
Expect this. Windows shows it for any downloaded program that is not code signed, and
Revert is not signed, because a signing certificate would publish the maintainer's legal
name. It is a warning about who we are, not about what the program does.

To continue: click "More info", then "Run anyway".

If you would rather verify before trusting, every release publishes a checksum:

  Get-FileHash revert-windows-amd64.zip -Algorithm SHA256

Compare it against revert-windows-amd64.zip.sha256 on the same release page. `revert
update` runs that same check automatically before it installs anything. Revert is open
source, so you can also read it or build it yourself:

  https://github.com/violetvandal/revert

If Defender ever reports a *named threat* rather than the unknown-publisher notice above,
that is a false positive and we want to hear about it. Please open an issue.

Quick start (command line)
--------------------------
Open a terminal in this folder:

  revert.exe doctor
  revert.exe setup
  revert.exe acquire-game-data --from "C:\path\to\your\THUG2"
  revert.exe build
  revert.exe run qol

Lanes:  run qol  (the modded edition) · run vanilla · run online (THUG Pro).
Options:  --glyphs xbox|playstation|gamecube|keyboard   --soundtrack original|radio

Controller
----------
Plug in an Xbox-style pad — sticks and face buttons work natively. For the PS2-style
shoulder combos THUG2 can't bind (get-off-board = LB+RB, rotate = LT/RT), vv-padbridge.exe
runs automatically during `run` and maps them to the game's keys. Play with the pad on.

Updating
--------
  revert.exe update --check     is there a newer release?
  revert.exe update             download it, replace the toolkit, rebuild

This downloads revert-windows-amd64.zip from the project's GitHub releases, checks it
against the published sha256, replaces the toolkit binaries in place, and rebuilds the
edition. Your game data, your built edition, and your Save\ folder are never touched. The
GUI has the same two buttons under "Update".

Afterwards, close and reopen revert-gui.exe so it runs the new version.

Keep machine-specific settings in revert.conf.local (next to revert.conf). An update
replaces revert.conf but never revert.conf.local; the outgoing revert.conf is kept as
revert.conf.bak.

THUG Pro (online)
-----------------
Install THUG Pro (THUGProSetup.exe) — it is a native Windows app. Then:  revert.exe run online

Optional extras (Python)
------------------------
The core edition needs no Python. For the cosmetic CAS extras (recolours, stickers,
licensed decks) install Python 3 from python.org and:  pip install pillow numpy
Then `revert.exe build` picks them up automatically.
