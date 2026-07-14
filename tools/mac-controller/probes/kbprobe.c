/* kbprobe — does the DirectInput KEYBOARD device see SendInput-injected numpad keys?
 * This is exactly the path THUG2 uses to read rotate/get-off-board (k0_* are DIK
 * scancodes) fed by the bridge's SendInput. We open a DInput keyboard the same way
 * the game does, inject KP7/KP9/KP1 with SendInput, and read them back via DInput.
 * Run it FROM ~/THUG2 with dinput8=n,b to exercise the proxy, and again without,
 * to prove whether the proxy interferes with keyboard input.
 * Build: i686-w64-mingw32-gcc -O2 -o kbprobe.exe kbprobe.c -ldinput8 -ldxguid -lole32 -luser32
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>

static void tap(WORD scan, IDirectInputDevice8A *kb, const char *nm, int dik){
    INPUT in; memset(&in,0,sizeof in);
    in.type=INPUT_KEYBOARD; in.ki.wScan=scan; in.ki.dwFlags=KEYEVENTF_SCANCODE;
    SendInput(1,&in,sizeof in);                 /* key DOWN */
    Sleep(40);
    BYTE keys[256]; memset(keys,0,sizeof keys);
    kb->lpVtbl->Poll(kb);
    HRESULT hr=kb->lpVtbl->GetDeviceState(kb, sizeof keys, keys);
    int seen = (SUCCEEDED(hr) && (keys[dik]&0x80));
    printf("  %s (DIK 0x%02x): DirectInput keyboard %s  (GetDeviceState hr=0x%08lx)\n",
           nm, dik, seen?"SAW IT  ✓":"did NOT see it  ✗", (unsigned long)hr);
    in.ki.dwFlags=KEYEVENTF_SCANCODE|KEYEVENTF_KEYUP;
    SendInput(1,&in,sizeof in);                 /* key UP */
    Sleep(40);
}

int main(void){
    IDirectInput8A *di=NULL; IDirectInputDevice8A *kb=NULL;
    if(FAILED(DirectInput8Create(GetModuleHandle(NULL),DIRECTINPUT_VERSION,
              &IID_IDirectInput8A,(void**)&di,NULL))){ printf("di create fail\n"); return 1; }
    if(FAILED(di->lpVtbl->CreateDevice(di,&GUID_SysKeyboard,&kb,NULL))){ printf("no keyboard device\n"); return 2; }
    kb->lpVtbl->SetDataFormat(kb,&c_dfDIKeyboard);
    kb->lpVtbl->SetCooperativeLevel(kb,GetDesktopWindow(),DISCL_BACKGROUND|DISCL_NONEXCLUSIVE);
    kb->lpVtbl->Acquire(kb);
    printf("KBPROBE: injecting numpad keys via SendInput, reading back via DirectInput keyboard...\n");
    tap(0x47, kb, "KP7", 0x47);   /* DIK_NUMPAD7 */
    tap(0x49, kb, "KP9", 0x49);   /* DIK_NUMPAD9 */
    tap(0x4f, kb, "KP1", 0x4f);   /* DIK_NUMPAD1 */
    printf("KBPROBE done.\n");
    return 0;
}
