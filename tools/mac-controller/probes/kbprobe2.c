/* kbprobe2 — like kbprobe but with a REAL foreground window (as the game has) and a
 * full DIK scan after each injection, so we can tell three cases apart:
 *   (a) key seen at expected DIK      -> SendInput->DInput works; bridge is viable
 *   (b) key seen at a DIFFERENT DIK   -> scancode/extended mapping issue (fixable)
 *   (c) no DIK set at all             -> SendInput never reaches DInput keyboard here
 * Build: i686-w64-mingw32-gcc -O2 -o kbprobe2.exe kbprobe2.c -ldinput8 -ldxguid -lole32 -luser32
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>

static IDirectInputDevice8A *kb;

static void pump(void){ MSG m; while(PeekMessageA(&m,NULL,0,0,PM_REMOVE)){ TranslateMessage(&m); DispatchMessageA(&m);} }

static void test(WORD scan, const char *nm){
    INPUT in; memset(&in,0,sizeof in);
    in.type=INPUT_KEYBOARD; in.ki.wScan=scan; in.ki.dwFlags=KEYEVENTF_SCANCODE;
    SendInput(1,&in,sizeof in);
    Sleep(30); pump();
    BYTE keys[256]; memset(keys,0,sizeof keys);
    kb->lpVtbl->Poll(kb);
    HRESULT hr=kb->lpVtbl->GetDeviceState(kb, sizeof keys, keys);
    printf("  inject %s (scan 0x%02x) hr=0x%08lx -> set DIK:", nm, scan, (unsigned long)hr);
    int any=0; for(int i=0;i<256;i++) if(keys[i]&0x80){ printf(" 0x%02x", i); any=1; }
    if(!any) printf(" (none)");
    printf("\n");
    in.ki.dwFlags=KEYEVENTF_SCANCODE|KEYEVENTF_KEYUP; SendInput(1,&in,sizeof in);
    Sleep(30); pump();
}

int main(void){
    WNDCLASSA wc; memset(&wc,0,sizeof wc); wc.lpfnWndProc=DefWindowProcA;
    wc.hInstance=GetModuleHandleA(NULL); wc.lpszClassName="kbprobe2";
    RegisterClassA(&wc);
    HWND hwnd=CreateWindowA("kbprobe2","kbprobe2",WS_OVERLAPPEDWINDOW|WS_VISIBLE,
                            0,0,320,200,NULL,NULL,wc.hInstance,NULL);
    ShowWindow(hwnd,SW_SHOW); SetForegroundWindow(hwnd); SetFocus(hwnd);
    for(int i=0;i<20;i++){ pump(); Sleep(20); }

    IDirectInput8A *di=NULL;
    if(FAILED(DirectInput8Create(GetModuleHandleA(NULL),DIRECTINPUT_VERSION,
              &IID_IDirectInput8A,(void**)&di,NULL))){ printf("di create fail\n"); return 1; }
    if(FAILED(di->lpVtbl->CreateDevice(di,&GUID_SysKeyboard,&kb,NULL))){ printf("no keyboard\n"); return 2; }
    kb->lpVtbl->SetDataFormat(kb,&c_dfDIKeyboard);
    kb->lpVtbl->SetCooperativeLevel(kb,hwnd,DISCL_FOREGROUND|DISCL_NONEXCLUSIVE);
    SetForegroundWindow(hwnd); SetFocus(hwnd); pump();
    HRESULT ah=kb->lpVtbl->Acquire(kb);
    printf("KBPROBE2: window=%p Acquire hr=0x%08lx  (DISCL_FOREGROUND keyboard)\n", (void*)hwnd, (unsigned long)ah);
    test(0x47,"KP7"); test(0x49,"KP9"); test(0x4f,"KP1");
    /* sanity: also inject a plain letter to confirm the pipe at all */
    test(0x10,"Q");
    printf("KBPROBE2 done.\n");
    return 0;
}
