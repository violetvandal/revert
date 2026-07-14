/* vv-dinput-bridge — macOS-lane DInput trigger bridge (with debug logging).
 * Reads the pad NON-EXCLUSIVELY (THUG2 keeps its own binding) and injects the
 * game's numpad keys via SendInput to recreate PS2 shoulder/trigger tricks:
 *   LB(btn4)/LT(lZ) -> KP7   RB(btn5)/RT(lRz) -> KP9   both SHOULDERS -> KP1 (get off board)
 *   (LT+RT together pass KP7+KP9 for acid drop / spine transfer — not stolen by get-off-board)
 * Build: i686-w64-mingw32-gcc -O2 -o vv-dinput-bridge.exe vv-dinput-bridge.c -ldinput8 -ldxguid -lole32 -luser32
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>
#define SCAN_KP7 0x47
#define SCAN_KP9 0x49
#define SCAN_KP1 0x4F
#define BTN_LB 4
#define BTN_RB 5
/* On this wine-mac pad BOTH triggers share ONE combined, center-resting axis (lZ);
 * lRz is dead (always 0). Idle lZ ~= 32768; LT drives it UP (~65408), RT drives it
 * DOWN (~128). So LT = lZ high side, RT = lZ low side, each with its own hysteresis.
 * (Consequence: LT+RT can't be read together on a combined axis — "level out" via
 * both triggers isn't possible here; each trigger alone works: nollie / switch+acid-drop.) */
#define LT_ON  52000   /* lZ >= this -> LT pressed  */
#define LT_OFF 44000   /* lZ <= this -> LT released */
#define RT_ON  12000   /* lZ <= this -> RT pressed  */
#define RT_OFF 20000   /* lZ >= this -> RT released */
static IDirectInput8A *g_di; static IDirectInputDevice8A *g_dev;
static BOOL CALLBACK dev_cb(LPCDIDEVICEINSTANCEA inst, LPVOID ctx){
    if (FAILED(g_di->lpVtbl->CreateDevice(g_di,&inst->guidInstance,&g_dev,NULL))) return DIENUM_CONTINUE;
    fprintf(stderr,"vv-dinput-bridge: using %s\n",inst->tszProductName); fflush(stderr); return DIENUM_STOP; }
static void send_key(WORD scan,BOOL down){
    INPUT in; memset(&in,0,sizeof in); in.type=INPUT_KEYBOARD; in.ki.wScan=scan;
    in.ki.dwFlags=KEYEVENTF_SCANCODE|(down?0:KEYEVENTF_KEYUP); SendInput(1,&in,sizeof in); }
/* high-side trigger (LT): pressed as the axis rises above ON, released below OFF */
static BOOL trigHi(BOOL prev,long v,long on,long off){ if(v>=on) return TRUE; if(v<=off) return FALSE; return prev; }
/* low-side trigger (RT): pressed as the axis falls below ON, released above OFF */
static BOOL trigLo(BOOL prev,long v,long on,long off){ if(v<=on) return TRUE; if(v>=off) return FALSE; return prev; }
int main(void){
    if(FAILED(DirectInput8Create(GetModuleHandle(NULL),DIRECTINPUT_VERSION,&IID_IDirectInput8A,(void**)&g_di,NULL))){fprintf(stderr,"di create fail\n");return 1;}
    g_di->lpVtbl->EnumDevices(g_di,DI8DEVCLASS_GAMECTRL,dev_cb,NULL,DIEDFL_ATTACHEDONLY);
    if(!g_dev){fprintf(stderr,"vv-dinput-bridge: no game controller\n");return 2;}
    g_dev->lpVtbl->SetDataFormat(g_dev,&c_dfDIJoystick2);
    g_dev->lpVtbl->SetCooperativeLevel(g_dev,GetDesktopWindow(),DISCL_BACKGROUND|DISCL_NONEXCLUSIVE);
    g_dev->lpVtbl->Acquire(g_dev);
    fprintf(stderr,"vv-dinput-bridge: LB/LT->KP7  RB/RT->KP9  both->KP1(walk)\n"); fflush(stderr);
    /* Rotate (KP7/KP9) is a WHILE-HELD action, so we track its down-state and hold
     * the key as long as the shoulder is held. Get-off-board (KP1) is a DISCRETE
     * action that THUG2 only registers as a short press+release, so we fire it as a
     * one-shot TAP on the rising edge of "both shoulders" (a long hold is ignored). */
    BOOL down7=0, down9=0; BOOL lt=0,rt=0; BOOL plb=0,prb=0,plt=0,prt=0; BOOL bothPrev=0;
    long vZ=0,vRz=0;
    for(;;){
        DIJOYSTATE2 s; memset(&s,0,sizeof s);
        g_dev->lpVtbl->Poll(g_dev);
        if(FAILED(g_dev->lpVtbl->GetDeviceState(g_dev,sizeof s,&s))){g_dev->lpVtbl->Acquire(g_dev);Sleep(20);continue;}
        /* log raw trigger axis values as they move, so a stuck/settled value is visible */
        if(labs(s.lZ-vZ)>3000 || labs(s.lRz-vRz)>3000){
            fprintf(stderr,"VAL lZ=%ld lRz=%ld\n",s.lZ,s.lRz); fflush(stderr); vZ=s.lZ; vRz=s.lRz;
        }
        lt=trigHi(lt,s.lZ,LT_ON,LT_OFF); rt=trigLo(rt,s.lZ,RT_ON,RT_OFF);
        BOOL lb=(s.rgbButtons[BTN_LB]&0x80)!=0, rb=(s.rgbButtons[BTN_RB]&0x80)!=0;
        /* log raw input transitions so we can SEE what the pad reports */
        if(lb!=plb){fprintf(stderr,"IN LB %s\n",lb?"down":"up");fflush(stderr);plb=lb;}
        if(rb!=prb){fprintf(stderr,"IN RB %s\n",rb?"down":"up");fflush(stderr);prb=rb;}
        if(lt!=plt){fprintf(stderr,"IN LT %s (lZ=%ld)\n",lt?"down":"up",s.lZ);fflush(stderr);plt=lt;}
        if(rt!=prt){fprintf(stderr,"IN RT %s (lRz=%ld)\n",rt?"down":"up",s.lRz);fflush(stderr);prt=rt;}
        BOOL left=lb||lt, right=rb||rt;
        /* get-off-board gesture = BOTH SHOULDERS only (matches the Win/Linux vv-padbridge).
         * Triggers-together (LT+RT) is deliberately NOT "both", so it passes KP7+KP9 through
         * for acid drop / spine transfer instead of being stolen by the get-off-board tap. */
        BOOL both=lb&&rb;
        /* rotate: hold while a single shoulder/trigger is held; suppressed during both-shoulders */
        BOOL want7=left&&!both, want9=right&&!both;
        if(want7!=down7){send_key(SCAN_KP7,want7);down7=want7;fprintf(stderr,"OUT KP7 %s\n",want7?"DOWN":"up");fflush(stderr);}
        if(want9!=down9){send_key(SCAN_KP9,want9);down9=want9;fprintf(stderr,"OUT KP9 %s\n",want9?"DOWN":"up");fflush(stderr);}
        /* get-off-board: one clean TAP on the rising edge of both-shoulders */
        if(both && !bothPrev){
            if(down7){send_key(SCAN_KP7,FALSE);down7=0;fprintf(stderr,"OUT KP7 up\n");}
            if(down9){send_key(SCAN_KP9,FALSE);down9=0;fprintf(stderr,"OUT KP9 up\n");}
            send_key(SCAN_KP1,TRUE);  fprintf(stderr,"OUT KP1 DOWN (tap)\n"); fflush(stderr);
            Sleep(60);
            send_key(SCAN_KP1,FALSE); fprintf(stderr,"OUT KP1 up (tap)\n");   fflush(stderr);
        }
        bothPrev=both;
        Sleep(8);
    }
    return 0;
}
