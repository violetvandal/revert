/* DInput pad probe for the macOS lane bridge design.
 * Prints button down/up (by index) and trigger-axis movement so we can map
 * LB/RB/LT/RT for the DInput trigger bridge. Reads NON-EXCLUSIVE/BACKGROUND so
 * it doesn't steal the pad from THUG2.
 * Build: i686-w64-mingw32-gcc -O2 -o padprobe.exe padprobe.c -ldinput8 -ldxguid -lole32 -luser32
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>

static IDirectInput8A *g_di;
static IDirectInputDevice8A *g_dev;

static BOOL CALLBACK dev_cb(LPCDIDEVICEINSTANCEA inst, LPVOID ctx){
    if (FAILED(g_di->lpVtbl->CreateDevice(g_di, &inst->guidInstance, &g_dev, NULL))) return DIENUM_CONTINUE;
    printf("device: %s\n", inst->tszProductName); fflush(stdout);
    return DIENUM_STOP;
}

int main(void){
    if (FAILED(DirectInput8Create(GetModuleHandle(NULL), DIRECTINPUT_VERSION,
                                  &IID_IDirectInput8A, (void**)&g_di, NULL))){ printf("di create fail\n"); return 1; }
    g_di->lpVtbl->EnumDevices(g_di, DI8DEVCLASS_GAMECTRL, dev_cb, NULL, DIEDFL_ATTACHEDONLY);
    if (!g_dev){ printf("no game controller\n"); return 2; }
    g_dev->lpVtbl->SetDataFormat(g_dev, &c_dfDIJoystick2);
    g_dev->lpVtbl->SetCooperativeLevel(g_dev, GetDesktopWindow(), DISCL_BACKGROUND|DISCL_NONEXCLUSIVE);
    g_dev->lpVtbl->Acquire(g_dev);
    printf("PROBE READY (~40s). Press ONE input at a time: LB, then RB, then LT, then RT.\n"); fflush(stdout);

    DIJOYSTATE2 prev; memset(&prev,0,sizeof prev);
    /* seed axis midpoints */
    for(int i=0;i<400;i++){
        DIJOYSTATE2 s; memset(&s,0,sizeof s);
        g_dev->lpVtbl->Poll(g_dev);
        if (FAILED(g_dev->lpVtbl->GetDeviceState(g_dev, sizeof s, &s))){ g_dev->lpVtbl->Acquire(g_dev); Sleep(50); continue; }
        for(int b=0;b<32;b++){
            if((s.rgbButtons[b]&0x80)!=(prev.rgbButtons[b]&0x80)){
                printf("BUTTON %d %s\n", b, (s.rgbButtons[b]&0x80)?"DOWN":"up"); fflush(stdout);
            }
        }
        long ax[6]={s.lX,s.lY,s.lZ,s.lRx,s.lRy,s.lRz};
        long pax[6]={prev.lX,prev.lY,prev.lZ,prev.lRx,prev.lRy,prev.lRz};
        const char* nm[6]={"lX","lY","lZ","lRx","lRy","lRz"};
        for(int a=0;a<6;a++){
            if(labs(ax[a]-pax[a])>4000){
                printf("AXIS %s = %ld\n", nm[a], ax[a]); fflush(stdout);
            }
        }
        prev=s;
        Sleep(60);
    }
    printf("--- probe done ---\n"); fflush(stdout);
    return 0;
}
