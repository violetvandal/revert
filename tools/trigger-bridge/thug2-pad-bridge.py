#!/usr/bin/env python3
"""
THUG2 pad bridge (dependency-free) — recreates the PS2 shoulder/trigger behaviour
THUG2's native DirectInput can't do under Wine, and provides the 2-button combos
the game can't bind (get-off-the-board = LB+RB).

Reads the Xbox pad straight from the Linux kernel (every input clean + independent)
and emits the game's keyboard keys through a uinput virtual keyboard. Unlike the
original tools/trigger-bridge/thug2-trigger-bridge.py this uses ONLY the Python 3
stdlib (raw evdev structs + raw uinput ioctls) so it runs on the Steam Deck with
nothing to install. It does NOT grab the pad, so Steam Input keeps driving the
sticks/face-buttons via its emulated gamepad at the same time.

THUG2 keyboard map (k0_*): KP7=Nollie/rotate-left, KP9=Switch/rotate-right,
KP1=get-off-board/walk.
  LT (L2) -> KP7 ;  RT (R2) -> KP9 ;  LT+RT -> Level Out (KP7+KP9)
  LB (L1) -> KP7 ;  RB (R1) -> KP9 ;  LB+RB -> KP1 get-off (suppresses the spins)
Sticks / A,B,X,Y / d-pad / Start stay native via DirectInput (the gp0_ binding).
"""
import os, sys, signal, struct, fcntl, ctypes, select, re

# ---- Linux input-subsystem constants ----
EV_SYN, EV_KEY, EV_ABS = 0x00, 0x01, 0x03
ABS_Z, ABS_RZ          = 0x02, 0x05        # left / right trigger axes (0..255, rest 0)
BTN_TL, BTN_TR         = 0x136, 0x137      # left / right bumper
KEY_KP7, KEY_KP9, KEY_KP1 = 71, 73, 79     # numpad 7 / 9 / 1
BUS_USB                = 0x03
TRIG_ON, TRIG_OFF      = 100, 60           # trigger press/release hysteresis

EV_FMT  = "@llHHi"                          # input_event: timeval(2*long) u16 u16 s32
EV_SIZE = struct.calcsize(EV_FMT)           # 24 on 64-bit

# ---- ioctl request builders (linux/uinput.h) ----
def _IOC(direction, typ, nr, size): return (direction << 30) | (size << 16) | (ord(typ) << 8) | nr
UI_SET_EVBIT   = _IOC(1, "U", 100, 4)       # _IOW('U',100,int)
UI_SET_KEYBIT  = _IOC(1, "U", 101, 4)       # _IOW('U',101,int)
UI_DEV_CREATE  = _IOC(0, "U", 1, 0)         # _IO('U',1)
UI_DEV_DESTROY = _IOC(0, "U", 2, 0)         # _IO('U',2)


def find_pad():
    """Return /dev/input/eventN for the Xbox-style pad, via /proc (no evdev dep)."""
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
        if any(s in name for s in ("X-Box", "Xbox", "360")):
            return "/dev/input/event" + h.group(1)
    return None


def make_uinput(keys):
    fd = os.open("/dev/uinput", os.O_WRONLY | os.O_NONBLOCK)
    fcntl.ioctl(fd, UI_SET_EVBIT, EV_KEY)
    fcntl.ioctl(fd, UI_SET_EVBIT, EV_SYN)
    for k in keys:
        fcntl.ioctl(fd, UI_SET_KEYBIT, k)
    # struct uinput_user_dev: name[80], input_id(4*u16), u32 ff_max, s32 abs[4][64]
    udev = struct.pack("<80sHHHHI256i", b"thug2-pad-bridge", BUS_USB, 0x045E, 0x028E, 1, 0,
                       *([0] * 256))
    os.write(fd, udev)
    fcntl.ioctl(fd, UI_DEV_CREATE)
    return fd


def main():
    path = find_pad()
    if not path:
        print("pad-bridge: no Xbox-style pad found", file=sys.stderr)
        return 1
    try:
        pad = os.open(path, os.O_RDONLY | os.O_NONBLOCK)
    except OSError as e:
        print(f"pad-bridge: cannot open {path}: {e}", file=sys.stderr)
        return 1
    ui = make_uinput([KEY_KP7, KEY_KP9, KEY_KP1])

    # die with our parent (the launcher, which becomes the wine game) so we never leak
    try:
        ctypes.CDLL("libc.so.6", use_errno=True).prctl(1, signal.SIGTERM)  # PR_SET_PDEATHSIG
    except Exception:
        pass

    st   = {"lt": False, "rt": False, "lb": False, "rb": False}
    down = {KEY_KP7: False, KEY_KP9: False, KEY_KP1: False}

    def emit(t, c, v):
        os.write(ui, struct.pack(EV_FMT, 0, 0, t, c, v))

    def hold(key, want):
        if down[key] != want:
            emit(EV_KEY, key, 1 if want else 0)
            emit(EV_SYN, 0, 0)
            down[key] = want

    def recompute():
        both = st["lb"] and st["rb"]
        hold(KEY_KP1, both)                                   # LB+RB -> get off board
        hold(KEY_KP7, st["lt"] or (st["lb"] and not both))    # nollie / spin-left
        hold(KEY_KP9, st["rt"] or (st["rb"] and not both))    # switch / spin-right

    def cleanup(*_):
        try:
            hold(KEY_KP7, False); hold(KEY_KP9, False); hold(KEY_KP1, False)
            fcntl.ioctl(ui, UI_DEV_DESTROY); os.close(ui)
        except Exception:
            pass
        sys.exit(0)

    signal.signal(signal.SIGINT, cleanup)
    signal.signal(signal.SIGTERM, cleanup)
    print(f"pad-bridge: {path}  LB+RB->get-off  LT/LB->KP7  RT/RB->KP9", file=sys.stderr, flush=True)

    buf = b""
    while True:
        r, _, _ = select.select([pad], [], [], 1.0)
        if not r:
            continue
        try:
            data = os.read(pad, EV_SIZE * 64)
        except BlockingIOError:
            continue
        except OSError:
            cleanup()
        if not data:
            continue
        buf += data
        while len(buf) >= EV_SIZE:
            chunk, buf = buf[:EV_SIZE], buf[EV_SIZE:]
            _, _, etype, code, value = struct.unpack(EV_FMT, chunk)
            if etype == EV_ABS:
                if code == ABS_Z:
                    st["lt"] = True if value >= TRIG_ON else (False if value <= TRIG_OFF else st["lt"])
                elif code == ABS_RZ:
                    st["rt"] = True if value >= TRIG_ON else (False if value <= TRIG_OFF else st["rt"])
                else:
                    continue
                recompute()
            elif etype == EV_KEY and value in (0, 1):
                if code == BTN_TL:
                    st["lb"] = bool(value); recompute()
                elif code == BTN_TR:
                    st["rb"] = bool(value); recompute()


if __name__ == "__main__":
    sys.exit(main())
