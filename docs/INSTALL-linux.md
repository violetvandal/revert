# Install on Linux (desktop)

A beginner-friendly walkthrough for a Linux PC or laptop. Works on **Fedora, Bazzite, Ubuntu/Debian,
and Arch/Manjaro**. One command does the whole thing.

> **You bring the game.** Revert is the installer and mods, not the game itself. You need to own
> Tony Hawk's Underground 2 (PC) and have your own copy ready as a folder or a download link.
>
> On a **Steam Deck**, use the click-installer guide instead: [INSTALL-steamdeck.md](INSTALL-steamdeck.md).

## What you need
- A 64-bit Linux desktop. Fedora is the most-tested; Bazzite, Ubuntu/Debian, and Arch/Manjaro are
  all supported.
- Your own THUG2 (PC) copy: either a download link, or the installed game folder on disk.
- About 15 to 30 minutes, most of it unattended download and build time.

## The one command
Open a terminal and paste this exactly:

```sh
bash <(curl -fsSL https://raw.githubusercontent.com/violetvandal/revert/main/install.sh)
```

> **Ubuntu or Debian?** Those ship without `curl`, so the command above stops with
> `curl: command not found`. Use this instead, which does exactly the same thing:
>
> ```sh
> bash <(wget -qO- https://raw.githubusercontent.com/violetvandal/revert/main/install.sh)
> ```
>
> (If you would rather stick with the first command, run `sudo apt install curl` first.)

> Paste it **exactly as shown** with the `bash <(curl …)` wrapper, **not** `curl … | bash`. The
> piped form cannot read your keyboard for the one password prompt.

That single command does everything below for you:

1. **Installs a few prerequisites** (git, the Go build tool, and curl if it is missing). It asks
   before installing system packages and needs your account password once. Say **yes**. Note that it
   can only do this once it is running, which is why fetching it needs either `curl` or `wget`
   already present.
2. **Downloads the toolkit** into `~/thug2`.
3. **Sets up Wine and your controller.** On a fresh, non-Steam-Deck machine it downloads a known-good
   Wine build automatically. Nothing is left half-configured.
4. **Asks for your game.** Paste your download link, or the path to your installed THUG2 folder.
5. **Builds the edition.** This is the long part; let it run.

When it finishes, it adds a **"THUG2: Violet Vandal Edition"** entry to your applications menu.

## Play

Launch it from your app menu, or from a terminal:

```sh
revert run qol
```

(`revert` is on your PATH after install, so you can run it from anywhere.) Other lanes:
`revert run vanilla` for a clean widescreen THUG2, and `revert run online` for THUG Pro (run
`revert setup --online` first).

## Distro notes
The command is the same everywhere; only the automatic package step differs under the hood.

- **Fedora / Bazzite:** the default and most-tested target. Bazzite's immutable system is handled
  automatically.
- **Ubuntu / Debian:** enables 32-bit (i386) support and pulls the apt equivalents automatically.
- **Arch / Manjaro:** enables the `multilib` repository and installs the 32-bit libraries
  automatically.

If package installation ever fails (for example a distro that renamed a package), run
`cd ~/thug2 && ./revert doctor`. It names exactly what is missing so you can install that one thing
and re-run the setup.

## Controller
Any pad in **XInput ("X") mode** works out of the box: an Xbox pad, an 8BitDo, a DualSense in X mode,
and most modern controllers. Most pads have a switch or a power-on button combo to pick XInput.

If your pad cannot do XInput (an older PlayStation pad in DInput mode, an arcade stick), bind it
yourself once:

```sh
revert configure-controller
```

This opens THUG2's own controller tool. Bind each control, Save, and close.

> The L2/R2 shoulder combos use a small helper that needs write access to `/dev/uinput`. If it is not
> writable, `revert setup` prints the one-line command to fix it, and `revert doctor` flags it.

## Naming a skater without a keyboard
If you play with a controller, THUG2's keyboard-only text screens are covered: when a text box opens,
**D-pad or left stick** cycles the letter, **A** commits, **X** backspaces, **Start** saves.

## Prefer clicking to typing?
Instead of the one-line command you can download the **graphical installer**
(`revert-installer-linux-amd64`) from the [releases page](https://github.com/violetvandal/revert/releases/latest),
and double-click it, choosing **Continue** when your file manager asks. It asks the same three
things and shows a live progress log. If your desktop refuses to run it (GNOME Files will not launch
programs this way), mark it executable first: right-click -> Properties -> Permissions -> Is
executable, or `chmod +x` it in a terminal.

## If something goes wrong
- `revert doctor` first. It pinpoints any missing dependency, prefix, or input.
- **No L2/R2 or no walk:** ensure `/dev/uinput` access (doctor warns and prints the fix).
- **Wrong buttons or pad not detected:** put it in XInput mode, or run `revert configure-controller`.
- Full manual/CLI reference: [INSTALL.md](INSTALL.md).
