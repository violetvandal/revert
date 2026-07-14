/*
 * dinput_probe_guid — print every attached game controller's DirectInput instance GUID.
 *
 * `revert calibrate-controller` runs this and writes the pad's guidInstance into the
 * registry as pad0. THUG2 (2004) opens ONLY the device whose guidInstance matches pad0, so
 * if that value is wrong the controller is simply dead — no error, no fallback.
 *
 * Why it has to be probed rather than shipped as a constant: these GUIDs are SYNTHESISED by
 * the driver stack, not burned into the pad. Wine derives them per-device/per-prefix (the
 * Steam Deck lane gets a different one from every fresh prefix), and macOS's wine hands out
 * yet another value. A hardcoded GUID works on exactly the machine it was captured from.
 *
 * The probe ENUMERATES ONLY — it never creates or acquires a device. Acquiring is what made
 * an earlier probe hang under newer wine, and a hang here would wedge `revert setup`.
 *
 * Output is parsed by parseGamepadGUID (internal/core/calibrate.go), one block per device:
 *
 *     device: Controller (XBOX 360 For Windows)
 *               devType=0x00010215 -> GAMEPAD (subtype 21) [HID]
 *               guidInstance=9E573EDE-7734-11D2-8D4A-23903FB6BDF7
 *
 * Build (32-bit, so it runs under wow64 on Windows and under wine on Linux/macOS alike):
 *   i686-w64-mingw32-gcc -O2 -o dinput_probe_guid.exe dinput_probe_guid.c \
 *       -ldinput8 -ldxguid -lole32 -luuid
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>

static int found = 0;

static const char *devtype_name(DWORD devtype)
{
    switch (devtype & 0xff) {
    case DI8DEVTYPE_GAMEPAD:        return "GAMEPAD";
    case DI8DEVTYPE_JOYSTICK:       return "JOYSTICK";
    case DI8DEVTYPE_DRIVING:        return "DRIVING";
    case DI8DEVTYPE_FLIGHT:         return "FLIGHT";
    case DI8DEVTYPE_1STPERSON:      return "1STPERSON";
    case DI8DEVTYPE_MOUSE:          return "MOUSE";
    case DI8DEVTYPE_KEYBOARD:       return "KEYBOARD";
    case DI8DEVTYPE_SUPPLEMENTAL:   return "SUPPLEMENTAL";
    default:                        return "OTHER";
    }
}

static BOOL CALLBACK enum_cb(LPCDIDEVICEINSTANCEA inst, LPVOID ctx)
{
    const GUID *g = &inst->guidInstance;
    (void)ctx;

    printf("device: %s\n", inst->tszProductName);
    printf("          devType=0x%08lx -> %s (subtype %lu)%s\n",
           (unsigned long)inst->dwDevType,
           devtype_name(inst->dwDevType),
           (unsigned long)((inst->dwDevType >> 8) & 0xff),
           (inst->dwDevType & DIDEVTYPE_HID) ? " [HID]" : "");
    /* No braces, uppercase: exactly the form THUG2 stores in pad0. */
    printf("          guidInstance=%08lX-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X\n",
           (unsigned long)g->Data1, g->Data2, g->Data3,
           g->Data4[0], g->Data4[1], g->Data4[2], g->Data4[3],
           g->Data4[4], g->Data4[5], g->Data4[6], g->Data4[7]);
    fflush(stdout);

    found++;
    return DIENUM_CONTINUE;
}

int main(void)
{
    LPDIRECTINPUT8A di = NULL;
    HRESULT hr;

    hr = DirectInput8Create(GetModuleHandleA(NULL), DIRECTINPUT_VERSION,
                            &IID_IDirectInput8A, (void **)&di, NULL);
    if (FAILED(hr)) {
        printf("DirectInput8Create failed (hr=0x%08lx)\n", (unsigned long)hr);
        return 1;
    }

    /* Game controllers only. THUG2 binds a pad, and enumerating the mouse/keyboard here
     * would only give the parser a chance to pick the wrong device. */
    hr = di->lpVtbl->EnumDevices(di, DI8DEVCLASS_GAMECTRL, enum_cb, NULL, DIEDFL_ATTACHEDONLY);
    if (FAILED(hr)) {
        printf("EnumDevices failed (hr=0x%08lx)\n", (unsigned long)hr);
        di->lpVtbl->Release(di);
        return 1;
    }
    di->lpVtbl->Release(di);

    if (!found) {
        printf("no game controller attached\n");
        return 2;
    }
    return 0;
}
