# THUG2 macOS lane — controller (Apple Silicon / wine-stable)

> **This is now packaged.** `revert setup` deploys all of it; `revert calibrate-controller`
> re-runs the binding step alone. See `internal/core/mac.go`.
>
> ⚠️ **pad0 is PROBED, never shipped.** An earlier draft of the packaged lane hardcoded the
> GUID captured from the dev Mac. That is wrong: wine *synthesises* the DirectInput instance
> GUID per device and per prefix (the Steam Deck gets a fresh one from every new prefix), and
> THUG2 opens only the device whose guidInstance equals pad0 — so a hardcoded value means a
> silently dead controller on anyone else's machine. `revert setup` runs
> `tools/xinput-probe/dinput_probe_guid.exe` under wine and writes the real GUID. The one in
> `tools/controls/thug2-settings-mac.reg` is just a placeholder that gets overwritten.

Makes an Xbox pad play THUG2 on the Mac lane **exactly like the Linux/Windows lanes** (same PC
game, same PS2-style controls + trigger tricks). Two pieces, both self-contained Windows PEs that
run under wine.

## Supported controllers — pair in XInput / "X" mode
The config (gp0 binding + XInput bridge) targets the **Xbox 360 controller layout**. Any controller
paired in **XInput mode** works unchanged, because wine's winexinput converts it to the "XBOX 360
For Windows" layout the config expects — this covers essentially every modern BT pad (Xbox, 8BitDo,
etc.; 8BitDo pads: pair in "X"/XInput mode). THUG2 (2004) binds only ONE controller layout at a
time and does not auto-adapt, so a pad left in its **native DirectInput mode** enumerates under its
own name (e.g. "8BitDo Ultimate 2 Wireless") with a totally different axis/button map — right
stick/triggers/buttons land on the wrong controls (stuck camera, wrong triggers) AND it's invisible
to the XInput bridge. Fix = re-pair it in XInput mode. (Per-device DInput re-binding is possible but
fiddly and controller-specific; not supported.)

## 1. `dinput8.c` / `dinput8.def` — left-stick de-inverter proxy
On the M1 + wine-stable, winexinput reports the **left-stick Y axis sign-reversed** vs. the
DirectInput convention THUG2 expects (push up → `lY=65535` instead of `0`). THUG2 has no invert
option, so this proxy `dinput8.dll` forwards every call to the real builtin dinput8 but **reflects
`GUID_YAxis` in `GetDeviceState`** (`lY' = (lMin+lMax) − lY`) before THUG2 reads it. It only wraps
GAMEPAD/JOYSTICK devices (mouse/keyboard also have a Y axis and must not be flipped) and learns the
axis range live via `DIPROP_RANGE`. Only `lY` (left stick) is touched.

- Build (32-bit):
  `i686-w64-mingw32-gcc -O2 -shared -o dinput8.dll dinput8.c dinput8.def -ldinput8 -ldxguid -lole32 -luuid -luser32`
- Deploy: put `dinput8.dll` in the game dir, copy wine's builtin dinput8 to `dinput8_real.dll`
  in the game dir, and add `dinput8=n,b` to `WINEDLLOVERRIDES`.

## 2. Trigger bridge — use `vv-padbridge` (XInput), the SAME binary as the Windows lane
Recreates the PS2 shoulder/trigger tricks THUG2 can't bind natively, injecting the game's numpad
keys via `SendInput`. **Use `cmd/vv-padbridge` (XInput) unchanged** — no Mac-specific bridge needed.

The catch that took a while to find: wine's XInput only sees the pad when the DirectInput
`override` key is **absent** (see §Registry). With it absent, XInput exposes the two triggers as
**separate** inputs. DirectInput instead **combines** them onto one shared axis (LT drives it up,
RT down, so LT+RT cancel and "level out" is impossible) — so XInput is required for the full scheme.

Mapping (identical to all lanes — the console scheme):
- **L1/R1** (shoulders) → KP7/KP9 (spin);  **L1+R1 → KP1** (get off board)
- **L2/R2** (triggers)  → KP7/KP9 (nollie/switch; R2 alone = acid drop);  **L2+R2 → KP7+KP9** (level out)

Build for wine (amd64 PE, runs under wow64): `GOOS=windows GOARCH=amd64 go build -o vv-padbridge.exe ./cmd/vv-padbridge`

`vv-dinput-bridge.c` in this folder is a **superseded DirectInput fallback** (kept for its
combined-trigger-axis RE) — only relevant if a future wine breaks the XInput path.

## 3. `vv-run.bat` — launch bridge + game in ONE virtual desktop (critical)
`SendInput` is **wine-desktop-scoped**: a bridge running on the default desktop cannot inject into
the game's virtual desktop (`explorer /desktop=thug2`). `vv-run.bat` starts the bridge and then the
game from the same batch, and is itself run via
`wine explorer /desktop=thug2,1440x900 cmd /c vv-run.bat`, so the bridge is a descendant of the
game's desktop process tree → injection reaches the game. It also `taskkill`s the bridge when the
game exits (otherwise the bridge's loop keeps the virtual desktop alive and the game can't relaunch).

## Registry / binding notes (per-machine, written by THUG2's Launcher.exe on the Mac)
- `pad0` = this Mac's device GUID (e.g. `9E573EDE-7734-11D2-8D4A-23903FB6BDF7`), NOT the canonical
  `...EDF`. The Launcher re-detects/writes it.
- Stick/button binding = the Launcher's Mac object order. Trick slots `gp0_14/15/16/17/19` stay
  UNBOUND (`0x0`) — delivered by the bridge as keyboard. `k0_14=0x4f`(KP1), `k0_16=0x47`(KP7),
  `k0_17=0x49`(KP9) must be intact.
- 🧱 **Do NOT set the wine `Software\Wine\DirectInput\Joysticks ..="override"` key on Mac.** Two
  reasons: (1) it shifts the DInput object order and breaks the whole binding; (2) it hides the pad
  from wine's **XInput**, which is what the trigger bridge needs for separate triggers. With the key
  absent, THUG2's DInput binding works AND XInput sees the pad. (Linux needs the key; Mac must not.)

## `probes/` — diagnostic kit (build like the bridge)
`padprobe.c` (pad axis/button reporter), `kbprobe.c` / `kbprobe2.c` / `kbxproc.c` (SendInput →
DirectInput-keyboard + cross-desktop injection tests). These are what proved the desktop-scoping
root cause; keep them for future controller debugging.

See memory `project_macos_lane` for the full story. The Linux/Windows equivalents:
`tools/trigger-bridge/` (evdev) and `cmd/vv-padbridge/` (XInput).
