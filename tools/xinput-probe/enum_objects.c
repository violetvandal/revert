/* Enumerate the 360 pad's DirectInput objects (axes/buttons/POVs) with their
 * instance numbers, to decode how THUG2's launcher numbers inputs in gp0_* regkeys.
 * Build: i686-w64-mingw32-gcc -O2 -o enum_objects.exe enum_objects.c -ldinput8 -ldxguid -lole32
 * Run:   WINEPREFIX=~/.wine-thug2-ge <ge>/bin/wine enum_objects.exe
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>

static IDirectInput8A *g_di;
static IDirectInputDevice8A *g_dev;

static BOOL CALLBACK obj_cb(LPCDIDEVICEOBJECTINSTANCEA o, LPVOID ctx)
{
    const char *kind = "?";
    if (o->dwType & DIDFT_AXIS)   kind = "AXIS";
    else if (o->dwType & DIDFT_BUTTON) kind = "BUTTON";
    else if (o->dwType & DIDFT_POV)    kind = "POV";
    /* instance number is encoded in dwType */
    unsigned inst = DIDFT_GETINSTANCE(o->dwType);
    printf("  %-6s inst=%-2u dwOfs=0x%02lx  \"%s\"\n", kind, inst, o->dwOfs, o->tszName);
    (void)ctx;
    return DIENUM_CONTINUE;
}

static BOOL CALLBACK dev_cb(LPCDIDEVICEINSTANCEA inst, LPVOID ctx)
{
    printf("Device: \"%s\"  (creating + enumerating objects)\n", inst->tszInstanceName);
    HRESULT hr = g_di->lpVtbl->CreateDevice(g_di, &inst->guidInstance, &g_dev, NULL);
    if (FAILED(hr) || !g_dev) { printf("  CreateDevice failed 0x%08lx\n", hr); return DIENUM_CONTINUE; }
    printf("--- AXES ---\n");
    g_dev->lpVtbl->EnumObjects(g_dev, obj_cb, NULL, DIDFT_AXIS);
    printf("--- POVs ---\n");
    g_dev->lpVtbl->EnumObjects(g_dev, obj_cb, NULL, DIDFT_POV);
    printf("--- BUTTONS ---\n");
    g_dev->lpVtbl->EnumObjects(g_dev, obj_cb, NULL, DIDFT_BUTTON);
    g_dev->lpVtbl->Release(g_dev); g_dev = NULL;
    (void)ctx;
    return DIENUM_CONTINUE;
}

int main(void)
{
    HRESULT hr = DirectInput8Create(GetModuleHandle(NULL), DIRECTINPUT_VERSION,
                                    &IID_IDirectInput8A, (void**)&g_di, NULL);
    if (FAILED(hr)) { printf("DirectInput8Create failed 0x%08lx\n", hr); return 1; }
    g_di->lpVtbl->EnumDevices(g_di, DI8DEVCLASS_GAMECTRL, dev_cb, NULL, DIEDFL_ATTACHEDONLY);
    g_di->lpVtbl->Release(g_di);
    printf("--- done ---\n");
    return 0;
}
