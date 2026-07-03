/* DirectInput enumeration probe — replicates THUG2's controller discovery.
 * Calls IDirectInput8::EnumDevices for game controllers (and all devices)
 * and prints each device's name, type, and GUID. Tells us whether the pad
 * is visible to DirectInput under Wine (and whether Override/registry tweaks
 * make it appear) WITHOUT launching the game or any GUI.
 *
 * Build (32-bit): i686-w64-mingw32-gcc -O2 -o dinput_probe.exe dinput_probe.c -ldinput8 -ldxguid -lole32
 * Run: WINEPREFIX=~/.wine-thug2-ge <ge>/bin/wine dinput_probe.exe
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>

static const char* devtype_name(DWORD t)
{
    switch (GET_DIDEVICE_TYPE(t)) {
        case DI8DEVTYPE_MOUSE:       return "MOUSE";
        case DI8DEVTYPE_KEYBOARD:    return "KEYBOARD";
        case DI8DEVTYPE_JOYSTICK:    return "JOYSTICK";
        case DI8DEVTYPE_GAMEPAD:     return "GAMEPAD";
        case DI8DEVTYPE_DRIVING:     return "DRIVING";
        case DI8DEVTYPE_FLIGHT:      return "FLIGHT";
        case DI8DEVTYPE_1STPERSON:   return "1STPERSON";
        case DI8DEVTYPE_DEVICECTRL:  return "DEVICECTRL";
        case DI8DEVTYPE_SCREENPOINTER:return "SCREENPOINTER";
        case DI8DEVTYPE_REMOTE:      return "REMOTE";
        case DI8DEVTYPE_SUPPLEMENTAL:return "SUPPLEMENTAL";
        default:                     return "OTHER";
    }
}

static void print_guid(const char *label, const GUID *g)
{
    printf("          %s=%08lX-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X\n",
           label, g->Data1, g->Data2, g->Data3,
           g->Data4[0], g->Data4[1], g->Data4[2], g->Data4[3],
           g->Data4[4], g->Data4[5], g->Data4[6], g->Data4[7]);
}

static BOOL CALLBACK enum_cb(LPCDIDEVICEINSTANCEA inst, LPVOID ctx)
{
    DWORD t = inst->dwDevType;
    printf("  DEVICE: \"%s\" (product \"%s\")\n", inst->tszInstanceName, inst->tszProductName);
    printf("          devType=0x%08lx -> %s (subtype %lu)%s\n",
           t, devtype_name(t), GET_DIDEVICE_SUBTYPE(t),
           (t & DIDEVTYPE_HID) ? " [HID]" : "");
    print_guid("guidInstance", &inst->guidInstance);
    print_guid("guidProduct ", &inst->guidProduct);
    (void)ctx;
    return DIENUM_CONTINUE;
}

int main(void)
{
    IDirectInput8A *di = NULL;
    HRESULT hr = DirectInput8Create(GetModuleHandle(NULL), DIRECTINPUT_VERSION,
                                    &IID_IDirectInput8A, (void**)&di, NULL);
    printf("DirectInput8Create hr=0x%08lx di=%p\n", hr, (void*)di);
    if (FAILED(hr) || !di) return 1;

    printf("\n=== EnumDevices(DI8DEVCLASS_GAMECTRL, ATTACHEDONLY) ===\n");
    hr = di->lpVtbl->EnumDevices(di, DI8DEVCLASS_GAMECTRL, enum_cb, NULL, DIEDFL_ATTACHEDONLY);
    printf("  (EnumDevices GAMECTRL hr=0x%08lx)\n", hr);

    printf("\n=== EnumDevices(DI8DEVCLASS_ALL, ATTACHEDONLY) ===\n");
    hr = di->lpVtbl->EnumDevices(di, DI8DEVCLASS_ALL, enum_cb, NULL, DIEDFL_ATTACHEDONLY);
    printf("  (EnumDevices ALL hr=0x%08lx)\n", hr);

    printf("\n=== EnumDevices(DI8DEVCLASS_GAMECTRL, ALLDEVICES) ===\n");
    hr = di->lpVtbl->EnumDevices(di, DI8DEVCLASS_GAMECTRL, enum_cb, NULL, DIEDFL_ALLDEVICES);
    printf("  (EnumDevices GAMECTRL ALLDEVICES hr=0x%08lx)\n", hr);

    di->lpVtbl->Release(di);
    printf("\n--- done ---\n");
    return 0;
}
