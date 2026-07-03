/* Live DInput state probe for the overridden 360 pad.
 * Acquires the device with the standard DIJOYSTATE2 format (same as the game
 * would) and prints which axis fields move — so we can see if the left stick
 * bleeds into the trigger axis slots (the level-off crosstalk bug).
 * Build: i686-w64-mingw32-gcc -O2 -o dinput_state.exe dinput_state.c -ldinput8 -ldxguid -lole32
 * Run:   WINEPREFIX=~/.wine-thug2-ge <ge>/bin/wine dinput_state.exe
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>

static IDirectInput8A *g_di;
static IDirectInputDevice8A *g_dev;

static BOOL CALLBACK dev_cb(LPCDIDEVICEINSTANCEA inst, LPVOID ctx)
{
    if (FAILED(g_di->lpVtbl->CreateDevice(g_di, &inst->guidInstance, &g_dev, NULL))) return DIENUM_CONTINUE;
    return DIENUM_STOP; /* take the first game controller */
}

static long center(long v){ return v - 32767; } /* show signed-ish deviation from mid */

int main(void)
{
    if (FAILED(DirectInput8Create(GetModuleHandle(NULL), DIRECTINPUT_VERSION,
                                  &IID_IDirectInput8A, (void**)&g_di, NULL))) return 1;
    g_di->lpVtbl->EnumDevices(g_di, DI8DEVCLASS_GAMECTRL, dev_cb, NULL, DIEDFL_ATTACHEDONLY);
    if (!g_dev) { printf("no game controller\n"); return 2; }

    g_dev->lpVtbl->SetDataFormat(g_dev, &c_dfDIJoystick2);
    g_dev->lpVtbl->SetCooperativeLevel(g_dev, GetDesktopWindow(), DISCL_BACKGROUND|DISCL_NONEXCLUSIVE);
    g_dev->lpVtbl->Acquire(g_dev);

    printf("Polling DIJOYSTATE2 for ~20s. Move ONE input at a time:\n");
    printf("  1) left stick only   2) left trigger only   3) right trigger only\n");
    printf("Watch which axis field reacts to each.\n\n");
    fflush(stdout);

    DIJOYSTATE2 prev; memset(&prev, 0, sizeof prev);
    for (int i = 0; i < 200; i++) {
        DIJOYSTATE2 s; memset(&s, 0, sizeof s);
        g_dev->lpVtbl->Poll(g_dev);
        if (FAILED(g_dev->lpVtbl->GetDeviceState(g_dev, sizeof s, &s))) { g_dev->lpVtbl->Acquire(g_dev); Sleep(50); continue; }
        /* print only when something meaningfully changed */
        if (labs(s.lX-prev.lX)>2000||labs(s.lY-prev.lY)>2000||labs(s.lZ-prev.lZ)>2000||
            labs(s.lRx-prev.lRx)>2000||labs(s.lRy-prev.lRy)>2000||labs(s.lRz-prev.lRz)>2000||
            s.rgdwPOV[0]!=prev.rgdwPOV[0]) {
            printf("lX=%6ld lY=%6ld lZ=%6ld | lRx=%6ld lRy=%6ld lRz=%6ld | POV=%lu\n",
                   center(s.lX), center(s.lY), center(s.lZ),
                   center(s.lRx), center(s.lRy), center(s.lRz), s.rgdwPOV[0]);
            fflush(stdout);
            prev = s;
        }
        Sleep(100);
    }
    printf("--- done ---\n");
    return 0;
}
