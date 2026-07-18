# Install on a Steam Deck

A beginner-friendly walkthrough. No terminal, no second computer, just your Deck. When you finish,
**Tony Hawk's Underground 2 (VV Edition)** shows up in your Steam library with full analog controls
and widescreen, ready to play.

> **You bring the game.** Revert is the installer and mods, not the game itself. You need to own
> Tony Hawk's Underground 2 (PC) and have your own copy ready as a folder or a download link.

## What you need
- A Steam Deck.
- Your own THUG2 (PC) copy: either a link you can paste, or the game folder on a USB drive / SD card.
- About 20 to 30 minutes (most of it is the Deck downloading and building in the background).

## Step by step

1. **Switch to Desktop Mode.** Hold the power button, choose **Switch to Desktop**.

2. **Download the installer.** Open Firefox and go to the releases page:
   `https://github.com/violetvandal/revert/releases/latest`
   Under **Assets**, download **`revert-installer-linux-amd64`**.

3. **Open it.** Open the **Dolphin** file manager, find the file in *Downloads*, and double-click
   it. Dolphin asks whether to launch it: choose **Launch**. (If it will not run, right-click it,
   choose **Properties -> Permissions**, tick **Is executable**, and try again.)

4. **Fill in three things.** A simple wizard opens in your browser and asks for:
   - **Where to install** (the default is fine).
   - **A password.** If your Deck has never had one, you **pick it here** and the wizard sets it up
     for you (at least 8 characters, not all numbers). Choose something you will remember.
   - **Your THUG2 copy.** Paste your download link, or point it at the game folder.
   Then press **Install & build**.

5. **Wait.** It installs Wine, downloads your game, builds the edition, and sets up the controller.
   A live log shows progress and the Deck stays awake. You do not need to touch anything.

6. **Play.** When it finishes it tells you to **switch back to Gaming Mode**. Do that, then open
   your **Library** and go to **Non-Steam games**, which is where the installer added it. Launch
   **"Tony Hawk's Underground 2 (VV Edition)"**. It has proper cover art, not a blank tile. That is it.

## Controller
Nothing to configure. Full analog sticks, widescreen, and PS2-style shoulder combos all work out of
the box. A quick reference:

| Deck control | THUG2 action |
|--------------|--------------|
| Left stick | Skate / move |
| Right stick | Camera |
| A / B / X / Y | Ollie / Grab / Flip / Grind |
| L1 / R1 | Spin left / right |
| **L1 + R1** | **Get off board / walk** |
| L2 / R2 | Nollie / Switch |
| **L2 + R2** | **Level out** |
| Start | Pause |

**Always play in Gaming Mode.** Desktop Mode loses controller focus on level loads.

## Naming a skater without a keyboard
THUG2's name and save-entry screens were keyboard-only. Revert adds controller text entry that turns
on automatically when a text box opens: **D-pad or left stick** cycles the letter, **A** commits it,
**X** backspaces, **Start** saves.

## If something goes wrong
- **No controller in-game:** make sure you launched it from your Steam library (not Desktop Mode).
- **Black screen right after launch:** rare wine mismatch. In Desktop Mode open Konsole and run
  `cd ~/thug2 && ./revert setup` once (game closed), then relaunch from Gaming Mode.
- Anything else: `cd ~/thug2 && ./revert doctor` prints exactly what is missing.

## Prefer to build on a PC and copy it over?
If you already run Revert on a Linux PC, you can build there and sync to the Deck instead. See the
**Advanced** section of [STEAMDECK.md](STEAMDECK.md).
