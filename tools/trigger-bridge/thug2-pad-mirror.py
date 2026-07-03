#!/usr/bin/env python3
"""
THUG2 pad MIRROR bridge (dependency-free) — isolates THUG2 from Steam Input's flaky
emulated pad, preserving full analog.

Problem: on the Steam Deck, Steam Input's emulated Xbox pad intermittently makes
Wine's DirectInput state go stale mid-game (level load / idle / heavy trigger use) —
the evdev node stays put and fed, but Wine stops handing the live state to THUG2's
DInput object, so analog input dies (keyboard keeps working). See memory
project_steamdeck_controller.

Fix: create ONE persistent virtual analog gamepad ("Violet Vandal Pad") via uinput
and continuously mirror Steam's emulated pad into it (re-finding Steam's pad if it
migrates). THUG2 binds to OUR pad (stable, steadily fed, non-Xbox VID/PID so Wine
treats it as a plain DInput HID gamepad with no winexinput layer). Steam's pad can
stall/recreate without the game ever seeing a change. Plus the keyboard combos
(LB+RB get-off, LT/LB->KP7, RT/RB->KP9) on a second uinput device, as before.

Stdlib only (raw evdev + raw uinput). Reads the source pad WITHOUT grabbing it.
"""
import os, sys, signal, struct, fcntl, ctypes, select, re, glob

# ---- evdev / input constants ----
EV_SYN, EV_KEY, EV_ABS = 0x00, 0x01, 0x03
ABS_X, ABS_Y, ABS_Z, ABS_RX, ABS_RY, ABS_RZ = 0, 1, 2, 3, 4, 5
ABS_HAT0X, ABS_HAT0Y = 0x10, 0x11
BTN_SOUTH, BTN_EAST, BTN_NORTH, BTN_WEST = 0x130, 0x131, 0x133, 0x134
BTN_TL, BTN_TR, BTN_SELECT, BTN_START, BTN_MODE, BTN_THUMBL, BTN_THUMBR = \
    0x136, 0x137, 0x13a, 0x13b, 0x13c, 0x13d, 0x13e
KEY_KP7, KEY_KP9, KEY_KP1 = 71, 73, 79

# the standard 11-button / 6-axis + hat xpad set we mirror (matches the gp0_ layout)
PAD_BTNS = [BTN_SOUTH, BTN_EAST, BTN_NORTH, BTN_WEST, BTN_TL, BTN_TR,
            BTN_SELECT, BTN_START, BTN_MODE, BTN_THUMBL, BTN_THUMBR]
STICK_AXES = [ABS_X, ABS_Y, ABS_RX, ABS_RY]
TRIG_AXES  = [ABS_Z, ABS_RZ]
HAT_AXES   = [ABS_HAT0X, ABS_HAT0Y]
ALL_AXES   = STICK_AXES + TRIG_AXES + HAT_AXES
TRIG_ON, TRIG_OFF = 100, 60
BUS_USB = 0x03

EV_FMT = "@llHHi"; EV_SIZE = struct.calcsize(EV_FMT)

def _IOC(d, t, nr, sz): return (d << 30) | (sz << 16) | (ord(t) << 8) | nr
UI_SET_EVBIT   = _IOC(1, "U", 100, 4)
UI_SET_KEYBIT  = _IOC(1, "U", 101, 4)
UI_SET_ABSBIT  = _IOC(1, "U", 103, 4)
UI_DEV_CREATE  = _IOC(0, "U", 1, 0)
UI_DEV_DESTROY = _IOC(0, "U", 2, 0)

VPAD_NAME = b"Violet Vandal Pad"   # MUST NOT contain "X-Box"/"360" (so we never mirror ourselves)


def make_vgamepad():
    fd = os.open("/dev/uinput", os.O_WRONLY | os.O_NONBLOCK)
    for ev in (EV_KEY, EV_ABS, EV_SYN):
        fcntl.ioctl(fd, UI_SET_EVBIT, ev)
    for b in PAD_BTNS:
        fcntl.ioctl(fd, UI_SET_KEYBIT, b)
    for a in ALL_AXES:
        fcntl.ioctl(fd, UI_SET_ABSBIT, a)
    absmax = [0] * 64; absmin = [0] * 64; absfuzz = [0] * 64; absflat = [0] * 64
    for a in STICK_AXES:
        absmin[a] = -32768; absmax[a] = 32767; absflat[a] = 128
    for a in TRIG_AXES:
        absmin[a] = 0; absmax[a] = 255
    for a in HAT_AXES:
        absmin[a] = -1; absmax[a] = 1
    # non-Xbox VID/PID -> Wine treats it as a plain DInput HID gamepad (no winexinput)
    udev = struct.pack("<80sHHHHI" + "64i" * 4, VPAD_NAME, BUS_USB, 0x1209, 0x764A, 0x0001, 0,
                       *absmax, *absmin, *absfuzz, *absflat)
    os.write(fd, udev)
    fcntl.ioctl(fd, UI_DEV_CREATE)
    return fd


def make_vkeyboard():
    fd = os.open("/dev/uinput", os.O_WRONLY | os.O_NONBLOCK)
    fcntl.ioctl(fd, UI_SET_EVBIT, EV_KEY); fcntl.ioctl(fd, UI_SET_EVBIT, EV_SYN)
    for k in (KEY_KP7, KEY_KP9, KEY_KP1):
        fcntl.ioctl(fd, UI_SET_KEYBIT, k)
    udev = struct.pack("<80sHHHHI" + "64i" * 4, b"Violet Vandal Combos", BUS_USB, 0x1209, 0x764B, 1, 0,
                       *([0] * 256))
    os.write(fd, udev)
    fcntl.ioctl(fd, UI_DEV_CREATE)
    return fd


def emit(fd, t, c, v):
    os.write(fd, struct.pack(EV_FMT, 0, 0, t, c, v))
    if t != EV_SYN:
        os.write(fd, struct.pack(EV_FMT, 0, 0, EV_SYN, 0, 0))


def find_source_pad():
    """current Steam emulated Xbox pad node (NOT our own virtual pad)."""
    try:
        blocks = open("/proc/bus/input/devices").read().split("\n\n")
    except OSError:
        return None
    for b in blocks:
        m = re.search(r'N: Name="([^"]*)"', b)
        name = m.group(1) if m else ""
        h = re.search(r"event(\d+)", b)
        if not h:
            continue
        if "Violet Vandal" in name:           # never mirror ourselves
            continue
        if any(s in name for s in ("X-Box", "Xbox", "360")):
            return "/dev/input/event" + h.group(1)
    return None


def main():
    vpad = make_vgamepad()
    vkbd = make_vkeyboard()
    try:
        ctypes.CDLL("libc.so.6", use_errno=True).prctl(1, signal.SIGTERM)  # die with parent
    except Exception:
        pass

    down = {KEY_KP7: False, KEY_KP9: False, KEY_KP1: False}
    st = {"lt": False, "rt": False, "lb": False, "rb": False}

    def hold(k, want):
        if down[k] != want:
            emit(vkbd, EV_KEY, k, 1 if want else 0); down[k] = want

    def combos():
        both = st["lb"] and st["rb"]
        hold(KEY_KP1, both)
        hold(KEY_KP7, st["lt"] or (st["lb"] and not both))
        hold(KEY_KP9, st["rt"] or (st["rb"] and not both))

    def cleanup(*_):
        for k in (KEY_KP7, KEY_KP9, KEY_KP1):
            try: emit(vkbd, EV_KEY, k, 0)
            except Exception: pass
        for fd in (vpad, vkbd):
            try: fcntl.ioctl(fd, UI_DEV_DESTROY); os.close(fd)
            except Exception: pass
        sys.exit(0)
    signal.signal(signal.SIGINT, cleanup); signal.signal(signal.SIGTERM, cleanup)

    src = None; src_path = None; buf = b""
    print("pad-mirror: virtual 'Violet Vandal Pad' created; waiting for Steam's pad…",
          file=sys.stderr, flush=True)

    while True:
        if src is None:
            src_path = find_source_pad()
            if src_path:
                try:
                    src = os.open(src_path, os.O_RDONLY | os.O_NONBLOCK); buf = b""
                    print(f"pad-mirror: mirroring {src_path}", file=sys.stderr, flush=True)
                except OSError:
                    src = None
            if src is None:
                # no source yet; poll
                if not select.select([], [], [], 1.0)[0]:
                    pass
                continue
        r, _, _ = select.select([src], [], [], 1.0)
        if not r:
            continue
        try:
            data = os.read(src, EV_SIZE * 64)
        except BlockingIOError:
            continue
        except OSError:
            os.close(src); src = None; continue        # source vanished -> re-find
        if not data:
            os.close(src); src = None; continue
        buf += data
        while len(buf) >= EV_SIZE:
            chunk, buf = buf[:EV_SIZE], buf[EV_SIZE:]
            _, _, etype, code, value = struct.unpack(EV_FMT, chunk)
            if etype == EV_ABS:
                if code in ALL_AXES:
                    emit(vpad, EV_ABS, code, value)        # mirror axis to our pad
                if code == ABS_Z:
                    st["lt"] = True if value >= TRIG_ON else (False if value <= TRIG_OFF else st["lt"]); combos()
                elif code == ABS_RZ:
                    st["rt"] = True if value >= TRIG_ON else (False if value <= TRIG_OFF else st["rt"]); combos()
            elif etype == EV_KEY:
                if code in PAD_BTNS and value in (0, 1):
                    emit(vpad, EV_KEY, code, value)        # mirror button to our pad
                if value in (0, 1):
                    if code == BTN_TL: st["lb"] = bool(value); combos()
                    elif code == BTN_TR: st["rb"] = bool(value); combos()


if __name__ == "__main__":
    sys.exit(main())
