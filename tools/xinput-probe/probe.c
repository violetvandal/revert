/* XInput probe — replicates how Clownjob'd loads XInput, to test whether
 * an app's XInputGetState sees the pad under Wine.
 *
 * Loads xinput1_4.dll the same way Clownjob'd does (LoadLibrary +
 * GetProcAddress), resolves both XInputGetState and the hidden ordinal-100
 * XInputGetStateEx (Guide-button variant Clownjob'd uses), then polls all 4
 * slots ~150 times (~15s) so you can press buttons / move sticks.
 *
 * Build (32-bit, matches the win32 prefix):
 *   i686-w64-mingw32-gcc -O2 -o probe.exe probe.c
 * Run in the SAME prefix as the game:
 *   WINEPREFIX=~/.wine-thug2-ge <ge>/bin/wine probe.exe
 */
#include <windows.h>
#include <stdio.h>

/* Minimal XInput state struct (avoids needing the xinput SDK header) */
typedef struct {
    unsigned short wButtons;
    unsigned char  bLeftTrigger;
    unsigned char  bRightTrigger;
    short sThumbLX, sThumbLY, sThumbRX, sThumbRY;
} XI_GAMEPAD;
typedef struct {
    unsigned long dwPacketNumber;
    XI_GAMEPAD Gamepad;
} XI_STATE;

typedef DWORD (WINAPI *PFN_GetState)(DWORD, XI_STATE*);

int main(void)
{
    HMODULE h = LoadLibraryA("xinput1_4.dll");
    printf("LoadLibrary(xinput1_4.dll) = %p\n", (void*)h);
    if (!h) { printf("FAILED to load xinput1_4.dll\n"); fflush(stdout); return 1; }

    PFN_GetState pGetState   = (PFN_GetState)GetProcAddress(h, "XInputGetState");
    /* ordinal 100 = XInputGetStateEx (undocumented, used for the Guide button) */
    PFN_GetState pGetStateEx = (PFN_GetState)GetProcAddress(h, (LPCSTR)100);
    printf("XInputGetState   = %p\n", (void*)pGetState);
    printf("XInputGetStateEx (ord 100) = %p\n", (void*)pGetStateEx);
    fflush(stdout);
    if (!pGetState) { printf("no XInputGetState export\n"); fflush(stdout); return 2; }

    printf("\n--- polling slots 0-3 for ~15s. PRESS BUTTONS / MOVE STICKS NOW ---\n");
    fflush(stdout);

    DWORD lastErr[4] = {0xFFFFFFFF,0xFFFFFFFF,0xFFFFFFFF,0xFFFFFFFF};
    for (int i = 0; i < 150; i++) {
        for (DWORD slot = 0; slot < 4; slot++) {
            XI_STATE st; memset(&st, 0, sizeof st);
            DWORD r = pGetState(slot, &st);
            if (r != lastErr[slot]) {
                if (r == ERROR_SUCCESS)
                    printf("slot %lu: CONNECTED (rc=0)\n", slot);
                else if (r == ERROR_DEVICE_NOT_CONNECTED)
                    printf("slot %lu: not connected (rc=1167)\n", slot);
                else
                    printf("slot %lu: rc=%lu\n", slot, r);
                lastErr[slot] = r;
                fflush(stdout);
            }
            if (r == ERROR_SUCCESS &&
                (st.Gamepad.wButtons || st.Gamepad.bLeftTrigger || st.Gamepad.bRightTrigger ||
                 st.Gamepad.sThumbLX > 8000 || st.Gamepad.sThumbLX < -8000 ||
                 st.Gamepad.sThumbLY > 8000 || st.Gamepad.sThumbLY < -8000)) {
                printf("slot %lu INPUT: btn=0x%04x LT=%u RT=%u LX=%d LY=%d RX=%d RY=%d\n",
                       slot, st.Gamepad.wButtons, st.Gamepad.bLeftTrigger, st.Gamepad.bRightTrigger,
                       st.Gamepad.sThumbLX, st.Gamepad.sThumbLY, st.Gamepad.sThumbRX, st.Gamepad.sThumbRY);
                fflush(stdout);
            }
        }
        Sleep(100);
    }
    printf("--- done ---\n");
    fflush(stdout);
    return 0;
}
