# Install on a Mac

A beginner-friendly walkthrough for macOS. Both **Apple Silicon** (M1 and later) and **Intel** Macs
are supported with the same one-line install. When you finish, **THUG2 Violet Vandal Edition** lives
in your Applications folder and launches from Spotlight like any other app.

> **You bring the game.** Revert is the installer and mods, not the game itself. You need to own
> Tony Hawk's Underground 2 (PC) and have your own copy ready as a folder or a download link.

## What you need
- A Mac running a recent macOS (Apple Silicon or Intel).
- Your own THUG2 (PC) copy: a download link, or the game folder on disk.
- About 15 to 30 minutes, most of it unattended.

**No Homebrew and no admin password.** The setup downloads a checksum-verified copy of Wine into its
own folder and installs nothing system-wide.

## Why Wine on a Mac?
THUG2 is a 32-bit Direct3D 9 game from 2004. Revert runs it under Wine and translates its graphics to
Metal through a build of DXVK we patched specifically for Macs (whose Metal backend has no geometry
shaders). The result is **GPU-accelerated**, not a software-rendered slideshow.

## Step by step

1. **Open Terminal.** It is in Applications -> Utilities, or press Command+Space and type "Terminal".

2. **Paste this one line** and press Return:

   ```sh
   bash <(curl -fsSL https://raw.githubusercontent.com/violetvandal/revert/main/install.sh)
   ```

3. **Command Line Tools prompt (first time only).** macOS ships a placeholder `git` that looks
   installed but is not. If you see Apple's "install the command line developer tools" dialog, click
   **Install** and wait for it to finish. This is a normal Apple system dialog. The installer waits
   for you.

4. **Answer the two prompts:** your account password (used once for setup), and your THUG2 copy
   (paste your link, or the folder path).

5. **Wait** while it downloads Wine, fetches your game, and builds the edition. A live log shows
   progress.

## Play

Launch **"THUG2 Violet Vandal Edition"** from Spotlight (Command+Space, start typing the name) or
from your **Applications** folder. Or run `revert run qol` in the Terminal.

### First launch: "Apple cannot verify this app"
Expected. Revert is not code-signed, because signing would publish the maintainer's legal name.
**Right-click** (or Control-click) the app in Applications, choose **Open**, then confirm **Open** in
the dialog. You only need to do this once. macOS then remembers your choice.

> **Prefer to click instead of the Terminal?** The graphical installer on the releases page is a
> `.zip`. Download it for your Mac (`revert-installer-darwin-arm64.zip` for Apple Silicon,
> `revert-installer-darwin-amd64.zip` for Intel), unzip it to get **Revert Installer**, then the
> first time **right-click the app -> Open -> confirm** (it is unsigned, so this clears Gatekeeper;
> a normal double-click works after that). The one-line Terminal install above skips the download
> and the quarantine entirely.

## Controller
Pair an **Xbox pad in XInput mode**. Note: macOS only exposes Microsoft-vendor pads to Wine, so other
brands will pair with macOS but stay invisible to the game. An Xbox controller is the reliable choice
on a Mac.

## Naming a skater without a keyboard
If you play with a controller, THUG2's keyboard-only text screens are covered: when a text box opens,
**D-pad or left stick** cycles the letter, **A** commits, **X** backspaces, **Start** saves.

## Good to know
- **Cutscenes** may show a mild judder on a 60Hz display. Gameplay is smooth. This is a frame-rate
  interaction in the pre-rendered movies, not a performance problem.
- The app bundle in `~/Applications` is yours to move or delete like any Mac app.

## Updating
```sh
revert update --check   # is a newer release available?
revert update           # update and rebuild (your saves are preserved)
```

## If something goes wrong
- `revert doctor` reports exactly what is present and what is next.
- **App will not open past the verify dialog:** use right-click -> **Open** (not a double-click) the
  first time.
- Full manual/CLI reference: [INSTALL.md](INSTALL.md).
