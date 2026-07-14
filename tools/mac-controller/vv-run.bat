@echo off
rem Launch the bridge AND the game in the SAME wine virtual desktop.
rem SendInput is desktop-scoped: a bridge on the default desktop cannot inject into
rem the game's virtual desktop. Running both here (bridge as a descendant of this
rem desktop's process tree) puts them on one desktop so injection reaches the game.
rem
rem BRIDGE = vv-padbridge.exe (XInput): on wine-mac, once the DirectInput "override"
rem key is absent, wine's XInput sees the pad and exposes the triggers SEPARATELY
rem (DirectInput combines them into one axis). XInput matches the Win/Linux lanes:
rem   L1/R1 -> KP7/KP9 (spin)   L1+R1 -> KP1 (get off board)
rem   L2/R2 -> KP7/KP9 (nollie/switch)   L2+R2 -> KP7+KP9 (level out)
cd /d "%~dp0"
start "vvbridge" /b cmd /c "vv-padbridge.exe > vv-bridge.log 2>&1"
THUG2.exe

rem Capture the GAME's exit code BEFORE taskkill overwrites ERRORLEVEL, and return it.
rem Without this the batch file exits with taskkill's status (always 0), so a crashed
rem game looked like a clean exit -- which hid a real crash during the .asi bisect and
rem defeated the caller's cold-start retry, which only fires on a non-zero exit.
set RC=%ERRORLEVEL%
taskkill /f /im vv-padbridge.exe >nul 2>&1
exit /b %RC%
