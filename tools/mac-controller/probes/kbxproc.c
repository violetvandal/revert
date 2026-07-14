/* kbxproc — cross-process SendInput test. Two modes:
 *   read   : create a foreground window, open a DInput keyboard (DISCL_FOREGROUND),
 *            poll for ~6s, report whether it EVER saw DIK_NUMPAD7 (0x47).
 *   inject : no window; after 1.5s, tap KP7 via SendInput ~40 times over ~4s.
 * Run BOTH in the SAME virtual desktop (explorer /desktop=shared) to prove whether a
 * windowless injector's SendInput reaches a windowed reader on that same desktop —
 * exactly the bridge->game relationship. Result written to a file (arg 2).
 * Build: i686-w64-mingw32-gcc -O2 -o kbxproc.exe kbxproc.c -ldinput8 -ldxguid -lole32 -luser32
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>

static void pump(void){ MSG m; while(PeekMessageA(&m,NULL,0,0,PM_REMOVE)){ TranslateMessage(&m); DispatchMessageA(&m);} }

static void do_inject(void){
    Sleep(1500);
    for(int i=0;i<40;i++){
        INPUT in; memset(&in,0,sizeof in); in.type=INPUT_KEYBOARD; in.ki.wScan=0x47;
        in.ki.dwFlags=KEYEVENTF_SCANCODE; SendInput(1,&in,sizeof in);
        Sleep(50);
        in.ki.dwFlags=KEYEVENTF_SCANCODE|KEYEVENTF_KEYUP; SendInput(1,&in,sizeof in);
        Sleep(50);
    }
}

static void do_read(const char *outpath){
    WNDCLASSA wc; memset(&wc,0,sizeof wc); wc.lpfnWndProc=DefWindowProcA;
    wc.hInstance=GetModuleHandleA(NULL); wc.lpszClassName="kbxproc";
    RegisterClassA(&wc);
    HWND hwnd=CreateWindowA("kbxproc","kbxproc",WS_OVERLAPPEDWINDOW|WS_VISIBLE,0,0,320,200,NULL,NULL,wc.hInstance,NULL);
    ShowWindow(hwnd,SW_SHOW); SetForegroundWindow(hwnd); SetFocus(hwnd);
    for(int i=0;i<20;i++){ pump(); Sleep(20); }
    IDirectInput8A *di=NULL; IDirectInputDevice8A *kb=NULL;
    DirectInput8Create(GetModuleHandleA(NULL),DIRECTINPUT_VERSION,&IID_IDirectInput8A,(void**)&di,NULL);
    di->lpVtbl->CreateDevice(di,&GUID_SysKeyboard,&kb,NULL);
    kb->lpVtbl->SetDataFormat(kb,&c_dfDIKeyboard);
    kb->lpVtbl->SetCooperativeLevel(kb,hwnd,DISCL_FOREGROUND|DISCL_NONEXCLUSIVE);
    SetForegroundWindow(hwnd); SetFocus(hwnd); pump();
    kb->lpVtbl->Acquire(kb);
    int seen=0; HRESULT lasthr=0;
    for(int t=0;t<120;t++){                 /* ~6s */
        pump();
        BYTE keys[256]; memset(keys,0,sizeof keys);
        kb->lpVtbl->Poll(kb);
        lasthr=kb->lpVtbl->GetDeviceState(kb,sizeof keys,keys);
        if(FAILED(lasthr)){ kb->lpVtbl->Acquire(kb); }
        if(keys[0x47]&0x80){ seen=1; break; }
        Sleep(50);
    }
    FILE *f=fopen(outpath,"w");
    if(f){ fprintf(f,"READER result: %s KP7  (last GetDeviceState hr=0x%08lx, foreground=%p)\n",
                   seen?"SAW":"never saw", (unsigned long)lasthr, (void*)GetForegroundWindow()); fclose(f); }
}

int main(int argc, char **argv){
    if(argc>=2 && !strcmp(argv[1],"inject")){ do_inject(); return 0; }
    if(argc>=3 && !strcmp(argv[1],"read")){ do_read(argv[2]); return 0; }
    printf("usage: kbxproc inject | kbxproc read <outfile>\n"); return 1;
}
