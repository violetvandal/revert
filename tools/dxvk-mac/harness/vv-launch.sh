#!/bin/bash
#
# Launch the game into the Mac's GUI session from a NON-GUI context (an SSH shell).
#
# Only needed for remote/dev iteration. `revert run qol` from Terminal.app, or the
# ~/Applications app bundle, is already in the GUI session and needs none of this.
#
# `launchctl asuser` hands the process to the logged-in user's GUI session (launchd), which
# is what puts a window on the physical screen. Do NOT use `open` here: it degrades after a
# few dozen launches (LaunchServices error -1712) and provokes the wine cold-start crash.
sudo launchctl asuser "$(id -u)" sudo -u "$(id -un)" "$HOME/THUG2.app/Contents/MacOS/THUG2" &
echo "launched (pid group $!)"
