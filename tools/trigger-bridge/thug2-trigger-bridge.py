#!/usr/bin/env python3
"""
THUG2 shoulder bridge — recreates the PS2 shoulder/trigger behavior that THUG2's
native DirectInput can't do under Wine (split triggers read as "always held",
and there's no native 2-button combo for walk).

Reads the Xbox pad directly from the Linux kernel (where every input is clean and
independent) and emits the game's keyboard keys via a uinput virtual keyboard.
THUG2's keyboard config maps: KP7=Nollie/Rotate-Left, KP9=Switch/Rotate-Right,
KP1=Get-off-board (walk toggle).

PS2-faithful mapping (per the THUG2 manual):
  LT  (L2) -> KP7  Nollie / rotate-left   (hold)
  RT  (R2) -> KP9  Switch / rotate-right / acid-drop  (hold)
  LT+RT    -> KP7+KP9 = Level Out / get out of halfpipe
  LB  (L1) -> KP7  spin left faster        (hold)
  RB  (R1) -> KP9  spin right faster       (hold)
  LB+RB    -> KP1  get off board / walk    (tap; suppresses the spin keys)

Everything else (sticks, A/B/X/Y, etc.) stays native analog via DirectInput.
Runs as the normal user (session ACLs grant /dev/input + /dev/uinput). No root.
"""
import os, sys, signal
import evdev
from evdev import ecodes, UInput

LT_AXIS, RT_AXIS = ecodes.ABS_Z, ecodes.ABS_RZ      # default triggers (XInput/xpad; rest 0)
LB_BTN,  RB_BTN  = ecodes.BTN_TL, ecodes.BTN_TR     # bumpers
KP7, KP9, KP1    = ecodes.KEY_KP7, ecodes.KEY_KP9, ecodes.KEY_KP1
TRIG_ON, TRIG_OFF = 100, 60                          # trigger hysteresis

def trigger_axes(dev):
    """Return (lt_axis, rt_axis): the axes that are the ACTUAL analog triggers on this pad.

    XInput/xpad pads put the triggers on ABS_Z/ABS_RZ (rest 0) and the right stick on
    ABS_RX/ABS_RY. But DirectInput-style HID pads (e.g. a GameSir G7 Pro in D-mode) do the
    opposite: the RIGHT STICK sits on ABS_Z/ABS_RZ (rest at centre ~128) and the triggers
    are ABS_GAS/ABS_BRAKE (rest 0). Reading Z/RZ on such a pad makes the centred stick look
    like both triggers are permanently held (jumps turn into acid-drops / level-outs). So
    prefer GAS/BRAKE whenever the pad exposes them; otherwise fall back to Z/RZ.
    """
    absc = {c for c, _ in dev.capabilities().get(ecodes.EV_ABS, [])}
    if {ecodes.ABS_GAS, ecodes.ABS_BRAKE} <= absc:
        return ecodes.ABS_BRAKE, ecodes.ABS_GAS      # LT = BRAKE, RT = GAS
    return LT_AXIS, RT_AXIS

def find_pad():
    # Identify the pad by CAPABILITY, not by name. The bridge needs both triggers as
    # axes (ABS_Z/ABS_RZ) and both bumpers (BTN_TL/BTN_TR); any XInput-style gamepad the
    # kernel binds via xpad exposes exactly that, regardless of brand. The old code
    # name-matched "Xbox"/"360", so it silently ignored 8BitDo (and every other
    # non-Microsoft XInput pad) — those enumerate under their own product name even
    # though their layout is identical. Prefer a Microsoft/Xbox-named pad when several
    # match (unchanged behaviour on setups that have one), else take the first capable one.
    picks = []
    for path in evdev.list_devices():
        try:
            d = evdev.InputDevice(path)
        except Exception:
            continue
        caps = d.capabilities()
        absc = {c for c, _ in caps.get(ecodes.EV_ABS, [])}
        keyc = set(caps.get(ecodes.EV_KEY, []))
        has_triggers = ({ecodes.ABS_Z, ecodes.ABS_RZ} <= absc
                        or {ecodes.ABS_GAS, ecodes.ABS_BRAKE} <= absc)
        if has_triggers and {ecodes.BTN_TL, ecodes.BTN_TR} <= keyc:
            picks.append(d)
    for d in picks:
        if any(s in d.name for s in ("X-Box", "Xbox", "360", "Microsoft X-Box")):
            return d
    return picks[0] if picks else None

def main():
    dev = find_pad()
    if not dev:
        print("shoulder-bridge: no XInput-style pad found (need trigger axes + bumpers)", file=sys.stderr)
        return 1
    lt_axis, rt_axis = trigger_axes(dev)
    ui = UInput({ecodes.EV_KEY: [KP7, KP9, KP1]}, name="thug2-shoulder-bridge")

    st = {"lt": False, "rt": False, "lb": False, "rb": False}
    keydown = {KP7: False, KP9: False, KP1: False}

    def hold(key, want):
        if keydown[key] != want:
            ui.write(ecodes.EV_KEY, key, 1 if want else 0); ui.syn()
            keydown[key] = want

    def recompute():
        both = st["lb"] and st["rb"]
        # LB+RB held -> hold KP1 (game toggles walk on the key-down edge)
        hold(KP1, both)
        # spin keys: triggers always; bumpers only when NOT doing the walk combo
        hold(KP7, st["lt"] or (st["lb"] and not both))
        hold(KP9, st["rt"] or (st["rb"] and not both))

    cleaning = False
    def cleanup(*_):
        # Release any held keys + close uinput, then exit HARD via os._exit so the
        # interpreter doesn't run evdev's __del__ deallocator during shutdown (that
        # races our teardown and prints an "exception ignored in deallocator"). The
        # guard makes a second signal — our launcher SIGTERMs then pkills — a no-op.
        nonlocal cleaning
        if not cleaning:
            cleaning = True
            try:
                hold(KP7, False); hold(KP9, False); hold(KP1, False); ui.close()
            except Exception:
                pass
        os._exit(0)
    signal.signal(signal.SIGINT, cleanup)
    signal.signal(signal.SIGTERM, cleanup)
    print(f"shoulder-bridge: {dev.name}  triggers={ecodes.ABS[lt_axis]}/{ecodes.ABS[rt_axis]}  "
          f"LT/LB->KP7  RT/RB->KP9  LB+RB->KP1(walk)  (ctrl-C to stop)",
          file=sys.stderr, flush=True)

    try:
        for ev in dev.read_loop():
            if ev.type == ecodes.EV_ABS:
                if ev.code == lt_axis:
                    st["lt"] = True if ev.value >= TRIG_ON else (False if ev.value <= TRIG_OFF else st["lt"])
                elif ev.code == rt_axis:
                    st["rt"] = True if ev.value >= TRIG_ON else (False if ev.value <= TRIG_OFF else st["rt"])
                else:
                    continue
                recompute()
            elif ev.type == ecodes.EV_KEY and ev.value in (0, 1):
                if ev.code == LB_BTN:   st["lb"] = bool(ev.value); recompute()
                elif ev.code == RB_BTN: st["rb"] = bool(ev.value); recompute()
    except OSError:
        cleanup()

if __name__ == "__main__":
    sys.exit(main())
