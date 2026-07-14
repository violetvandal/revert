/* dinput8.c — macOS-lane left-stick de-inverter proxy for THUG2.
 *
 * On this M1 + wine-stable, winexinput reports the left-stick Y axis with the
 * SIGN REVERSED vs. the DirectInput convention THUG2 expects (push UP -> lY=65535
 * instead of 0). There is no invert flag anywhere in THUG2's config to fix this,
 * so we intercept it at the source: a proxy dinput8.dll that forwards every call
 * to the real (builtin) dinput8, but reflects GUID_YAxis in GetDeviceState before
 * THUG2 reads it.
 *
 * Safety: only GAMEPAD/JOYSTICK devices are wrapped (mouse/keyboard also expose a
 * Y axis and must NOT be flipped). The Y offset is taken from THUG2's own data
 * format, and the reflection uses the device's actual axis range (queried live),
 * so it is correct regardless of the range THUG2 sets.
 *
 * Deploy: place this dinput8.dll in ~/THUG2, copy wine's builtin dinput8.dll to
 * ~/THUG2/dinput8_real.dll, and add  dinput8=n,b  to WINEDLLOVERRIDES.
 *
 * Build (32-bit):
 *   i686-w64-mingw32-gcc -O2 -shared -o dinput8.dll src/dinput8.c src/dinput8.def \
 *       -ldinput8 -ldxguid -lole32 -luuid -luser32
 */
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <stdio.h>
#include <stdlib.h>

/* ---- tiny log so the-core / debugging can SEE the proxy engage ---- */
static void plog(const char *fmt, ...){
    FILE *f = fopen("vv-dinput8-proxy.log", "a");
    if(!f) return;
    va_list ap; va_start(ap, fmt); vfprintf(f, fmt, ap); va_end(ap);
    fputc('\n', f); fclose(f);
}

/* ================= device wrapper ================= */
typedef struct {
    const IDirectInputDevice8AVtbl *lpVtbl;
    IDirectInputDevice8A *real;
    DWORD yOfs;      /* byte offset of lY inside the game's data format */
    LONG  invSum;    /* lMin+lMax for reflection; -1 = not yet learned  */
    int   haveY;     /* GUID_YAxis found in the data format             */
} DevWrap;

static const IDirectInputDevice8AVtbl g_devVtbl;

/* ---- the two methods we actually hook ---- */
static HRESULT STDMETHODCALLTYPE dev_SetDataFormat(IDirectInputDevice8A *This, LPCDIDATAFORMAT df){
    DevWrap *w=(DevWrap*)This;
    HRESULT hr = w->real->lpVtbl->SetDataFormat(w->real, df);
    if(SUCCEEDED(hr) && df){
        w->haveY=0;
        for(DWORD i=0;i<df->dwNumObjs;i++){
            const DIOBJECTDATAFORMAT *o=&df->rgodf[i];
            if(o->pguid && IsEqualGUID(o->pguid,&GUID_YAxis)){ w->yOfs=o->dwOfs; w->haveY=1; break; }
        }
        w->invSum=-1;   /* re-learn range against the new format */
        plog("SetDataFormat: haveY=%d yOfs=%lu", w->haveY, (unsigned long)w->yOfs);
    }
    return hr;
}

static HRESULT STDMETHODCALLTYPE dev_GetDeviceState(IDirectInputDevice8A *This, DWORD cb, LPVOID data){
    DevWrap *w=(DevWrap*)This;
    HRESULT hr = w->real->lpVtbl->GetDeviceState(w->real, cb, data);
    if(SUCCEEDED(hr) && w->haveY && data && w->yOfs + sizeof(LONG) <= cb){
        if(w->invSum < 0){
            DIPROPRANGE r; memset(&r,0,sizeof r);
            r.diph.dwSize=sizeof r; r.diph.dwHeaderSize=sizeof r.diph;
            r.diph.dwHow=DIPH_BYOFFSET; r.diph.dwObj=w->yOfs;
            if(SUCCEEDED(w->real->lpVtbl->GetProperty(w->real, DIPROP_RANGE, &r.diph)))
                w->invSum = r.lMin + r.lMax;
            else
                w->invSum = 65535;   /* wine default axis range 0..65535 */
            plog("learned invSum=%ld (reflect lY about center)", w->invSum);
        }
        LONG *py = (LONG*)((BYTE*)data + w->yOfs);
        *py = w->invSum - *py;
    }
    return hr;
}

/* ---- IUnknown ---- */
static HRESULT STDMETHODCALLTYPE dev_QueryInterface(IDirectInputDevice8A *This, REFIID riid, void **ppv){
    DevWrap *w=(DevWrap*)This;
    if(IsEqualIID(riid,&IID_IUnknown) || IsEqualIID(riid,&IID_IDirectInputDevice8A) || IsEqualIID(riid,&IID_IDirectInputDevice8W)){
        *ppv=This; This->lpVtbl->AddRef(This); return S_OK;
    }
    return w->real->lpVtbl->QueryInterface(w->real, riid, ppv);
}
static ULONG STDMETHODCALLTYPE dev_AddRef(IDirectInputDevice8A *This){
    DevWrap *w=(DevWrap*)This; return w->real->lpVtbl->AddRef(w->real);
}
static ULONG STDMETHODCALLTYPE dev_Release(IDirectInputDevice8A *This){
    DevWrap *w=(DevWrap*)This; ULONG rc=w->real->lpVtbl->Release(w->real);
    if(rc==0) free(w);
    return rc;
}

/* ---- everything else: straight forward to the real device ---- */
#define R DevWrap *w=(DevWrap*)This; return w->real->lpVtbl
static HRESULT STDMETHODCALLTYPE dev_GetCapabilities(IDirectInputDevice8A *This, LPDIDEVCAPS a){ R->GetCapabilities(w->real,a);}
static HRESULT STDMETHODCALLTYPE dev_EnumObjects(IDirectInputDevice8A *This, LPDIENUMDEVICEOBJECTSCALLBACKA a, LPVOID b, DWORD c){ R->EnumObjects(w->real,a,b,c);}
static HRESULT STDMETHODCALLTYPE dev_GetProperty(IDirectInputDevice8A *This, REFGUID a, LPDIPROPHEADER b){ R->GetProperty(w->real,a,b);}
static HRESULT STDMETHODCALLTYPE dev_SetProperty(IDirectInputDevice8A *This, REFGUID a, LPCDIPROPHEADER b){ R->SetProperty(w->real,a,b);}
static HRESULT STDMETHODCALLTYPE dev_Acquire(IDirectInputDevice8A *This){ R->Acquire(w->real);}
static HRESULT STDMETHODCALLTYPE dev_Unacquire(IDirectInputDevice8A *This){ R->Unacquire(w->real);}
static HRESULT STDMETHODCALLTYPE dev_GetDeviceData(IDirectInputDevice8A *This, DWORD a, LPDIDEVICEOBJECTDATA b, LPDWORD c, DWORD d){ R->GetDeviceData(w->real,a,b,c,d);}
static HRESULT STDMETHODCALLTYPE dev_SetEventNotification(IDirectInputDevice8A *This, HANDLE a){ R->SetEventNotification(w->real,a);}
static HRESULT STDMETHODCALLTYPE dev_SetCooperativeLevel(IDirectInputDevice8A *This, HWND a, DWORD b){ R->SetCooperativeLevel(w->real,a,b);}
static HRESULT STDMETHODCALLTYPE dev_GetObjectInfo(IDirectInputDevice8A *This, LPDIDEVICEOBJECTINSTANCEA a, DWORD b, DWORD c){ R->GetObjectInfo(w->real,a,b,c);}
static HRESULT STDMETHODCALLTYPE dev_GetDeviceInfo(IDirectInputDevice8A *This, LPDIDEVICEINSTANCEA a){ R->GetDeviceInfo(w->real,a);}
static HRESULT STDMETHODCALLTYPE dev_RunControlPanel(IDirectInputDevice8A *This, HWND a, DWORD b){ R->RunControlPanel(w->real,a,b);}
static HRESULT STDMETHODCALLTYPE dev_Initialize(IDirectInputDevice8A *This, HINSTANCE a, DWORD b, REFGUID c){ R->Initialize(w->real,a,b,c);}
static HRESULT STDMETHODCALLTYPE dev_CreateEffect(IDirectInputDevice8A *This, REFGUID a, LPCDIEFFECT b, LPDIRECTINPUTEFFECT *c, LPUNKNOWN d){ R->CreateEffect(w->real,a,b,c,d);}
static HRESULT STDMETHODCALLTYPE dev_EnumEffects(IDirectInputDevice8A *This, LPDIENUMEFFECTSCALLBACKA a, LPVOID b, DWORD c){ R->EnumEffects(w->real,a,b,c);}
static HRESULT STDMETHODCALLTYPE dev_GetEffectInfo(IDirectInputDevice8A *This, LPDIEFFECTINFOA a, REFGUID b){ R->GetEffectInfo(w->real,a,b);}
static HRESULT STDMETHODCALLTYPE dev_GetForceFeedbackState(IDirectInputDevice8A *This, LPDWORD a){ R->GetForceFeedbackState(w->real,a);}
static HRESULT STDMETHODCALLTYPE dev_SendForceFeedbackCommand(IDirectInputDevice8A *This, DWORD a){ R->SendForceFeedbackCommand(w->real,a);}
static HRESULT STDMETHODCALLTYPE dev_EnumCreatedEffectObjects(IDirectInputDevice8A *This, LPDIENUMCREATEDEFFECTOBJECTSCALLBACK a, LPVOID b, DWORD c){ R->EnumCreatedEffectObjects(w->real,a,b,c);}
static HRESULT STDMETHODCALLTYPE dev_Escape(IDirectInputDevice8A *This, LPDIEFFESCAPE a){ R->Escape(w->real,a);}
static HRESULT STDMETHODCALLTYPE dev_Poll(IDirectInputDevice8A *This){ R->Poll(w->real);}
static HRESULT STDMETHODCALLTYPE dev_SendDeviceData(IDirectInputDevice8A *This, DWORD a, LPCDIDEVICEOBJECTDATA b, LPDWORD c, DWORD d){ R->SendDeviceData(w->real,a,b,c,d);}
static HRESULT STDMETHODCALLTYPE dev_EnumEffectsInFile(IDirectInputDevice8A *This, LPCSTR a, LPDIENUMEFFECTSINFILECALLBACK b, LPVOID c, DWORD d){ R->EnumEffectsInFile(w->real,a,b,c,d);}
static HRESULT STDMETHODCALLTYPE dev_WriteEffectToFile(IDirectInputDevice8A *This, LPCSTR a, DWORD b, LPDIFILEEFFECT c, DWORD d){ R->WriteEffectToFile(w->real,a,b,c,d);}
static HRESULT STDMETHODCALLTYPE dev_BuildActionMap(IDirectInputDevice8A *This, LPDIACTIONFORMATA a, LPCSTR b, DWORD c){ R->BuildActionMap(w->real,a,b,c);}
static HRESULT STDMETHODCALLTYPE dev_SetActionMap(IDirectInputDevice8A *This, LPDIACTIONFORMATA a, LPCSTR b, DWORD c){ R->SetActionMap(w->real,a,b,c);}
static HRESULT STDMETHODCALLTYPE dev_GetImageInfo(IDirectInputDevice8A *This, LPDIDEVICEIMAGEINFOHEADERA a){ R->GetImageInfo(w->real,a);}
#undef R

static const IDirectInputDevice8AVtbl g_devVtbl = {
    dev_QueryInterface, dev_AddRef, dev_Release,
    dev_GetCapabilities, dev_EnumObjects, dev_GetProperty, dev_SetProperty,
    dev_Acquire, dev_Unacquire, dev_GetDeviceState, dev_GetDeviceData,
    dev_SetDataFormat, dev_SetEventNotification, dev_SetCooperativeLevel,
    dev_GetObjectInfo, dev_GetDeviceInfo, dev_RunControlPanel, dev_Initialize,
    dev_CreateEffect, dev_EnumEffects, dev_GetEffectInfo, dev_GetForceFeedbackState,
    dev_SendForceFeedbackCommand, dev_EnumCreatedEffectObjects, dev_Escape, dev_Poll,
    dev_SendDeviceData, dev_EnumEffectsInFile, dev_WriteEffectToFile,
    dev_BuildActionMap, dev_SetActionMap, dev_GetImageInfo
};

/* ================= IDirectInput8 wrapper ================= */
typedef struct { const IDirectInput8AVtbl *lpVtbl; IDirectInput8A *real; } DiWrap;
static const IDirectInput8AVtbl g_diVtbl;

static HRESULT STDMETHODCALLTYPE di_CreateDevice(IDirectInput8A *This, REFGUID rguid, LPDIRECTINPUTDEVICE8A *out, LPUNKNOWN punk){
    DiWrap *w=(DiWrap*)This;
    IDirectInputDevice8A *rdev=NULL;
    HRESULT hr=w->real->lpVtbl->CreateDevice(w->real, rguid, &rdev, punk);
    if(FAILED(hr) || !rdev){ if(out)*out=rdev; return hr; }

    DIDEVCAPS caps; memset(&caps,0,sizeof caps); caps.dwSize=sizeof caps;
    DWORD t=0;
    if(SUCCEEDED(rdev->lpVtbl->GetCapabilities(rdev,&caps))) t=GET_DIDEVICE_TYPE(caps.dwDevType);

    if(t==DI8DEVTYPE_JOYSTICK || t==DI8DEVTYPE_GAMEPAD || t==DI8DEVTYPE_DRIVING ||
       t==DI8DEVTYPE_FLIGHT   || t==DI8DEVTYPE_1STPERSON){
        DevWrap *dw=(DevWrap*)calloc(1,sizeof *dw);
        dw->lpVtbl=&g_devVtbl; dw->real=rdev; dw->invSum=-1; dw->haveY=0;
        *out=(IDirectInputDevice8A*)dw;
        plog("CreateDevice: wrapping gamepad/joystick (devtype=0x%lx)", (unsigned long)t);
    } else {
        *out=rdev;   /* mouse/keyboard/etc: pass through untouched */
    }
    return hr;
}

#define Q DiWrap *w=(DiWrap*)This; return w->real->lpVtbl
static HRESULT STDMETHODCALLTYPE di_QueryInterface(IDirectInput8A *This, REFIID riid, void **ppv){
    DiWrap *w=(DiWrap*)This;
    if(IsEqualIID(riid,&IID_IUnknown) || IsEqualIID(riid,&IID_IDirectInput8A) || IsEqualIID(riid,&IID_IDirectInput8W)){
        *ppv=This; This->lpVtbl->AddRef(This); return S_OK;
    }
    return w->real->lpVtbl->QueryInterface(w->real, riid, ppv);
}
static ULONG STDMETHODCALLTYPE di_AddRef(IDirectInput8A *This){ DiWrap *w=(DiWrap*)This; return w->real->lpVtbl->AddRef(w->real);}
static ULONG STDMETHODCALLTYPE di_Release(IDirectInput8A *This){ DiWrap *w=(DiWrap*)This; ULONG rc=w->real->lpVtbl->Release(w->real); if(rc==0) free(w); return rc;}
static HRESULT STDMETHODCALLTYPE di_EnumDevices(IDirectInput8A *This, DWORD a, LPDIENUMDEVICESCALLBACKA b, LPVOID c, DWORD d){ Q->EnumDevices(w->real,a,b,c,d);}
static HRESULT STDMETHODCALLTYPE di_GetDeviceStatus(IDirectInput8A *This, REFGUID a){ Q->GetDeviceStatus(w->real,a);}
static HRESULT STDMETHODCALLTYPE di_RunControlPanel(IDirectInput8A *This, HWND a, DWORD b){ Q->RunControlPanel(w->real,a,b);}
static HRESULT STDMETHODCALLTYPE di_Initialize(IDirectInput8A *This, HINSTANCE a, DWORD b){ Q->Initialize(w->real,a,b);}
static HRESULT STDMETHODCALLTYPE di_FindDevice(IDirectInput8A *This, REFGUID a, LPCSTR b, LPGUID c){ Q->FindDevice(w->real,a,b,c);}
static HRESULT STDMETHODCALLTYPE di_EnumDevicesBySemantics(IDirectInput8A *This, LPCSTR a, LPDIACTIONFORMATA b, LPDIENUMDEVICESBYSEMANTICSCBA c, LPVOID d, DWORD e){ Q->EnumDevicesBySemantics(w->real,a,b,c,d,e);}
static HRESULT STDMETHODCALLTYPE di_ConfigureDevices(IDirectInput8A *This, LPDICONFIGUREDEVICESCALLBACK a, LPDICONFIGUREDEVICESPARAMSA b, DWORD c, LPVOID d){ Q->ConfigureDevices(w->real,a,b,c,d);}
#undef Q

static const IDirectInput8AVtbl g_diVtbl = {
    di_QueryInterface, di_AddRef, di_Release,
    di_CreateDevice, di_EnumDevices, di_GetDeviceStatus, di_RunControlPanel,
    di_Initialize, di_FindDevice, di_EnumDevicesBySemantics, di_ConfigureDevices
};

/* ================= exported entry point ================= */
typedef HRESULT (WINAPI *DI8CREATE)(HINSTANCE, DWORD, REFIID, LPVOID*, LPUNKNOWN);
static DI8CREATE real_create;

static BOOL load_real(void){
    if(real_create) return TRUE;
    HMODULE h=LoadLibraryA("dinput8_real.dll");
    if(!h){ plog("FATAL: cannot load dinput8_real.dll (err=%lu)", (unsigned long)GetLastError()); return FALSE; }
    real_create=(DI8CREATE)GetProcAddress(h,"DirectInput8Create");
    if(!real_create){ plog("FATAL: no DirectInput8Create in dinput8_real.dll"); return FALSE; }
    return TRUE;
}

HRESULT WINAPI DirectInput8Create(HINSTANCE hinst, DWORD ver, REFIID riid, LPVOID *ppv, LPUNKNOWN punk){
    if(!load_real()) return E_FAIL;
    HRESULT hr=real_create(hinst, ver, riid, ppv, punk);
    if(FAILED(hr) || !ppv || !*ppv) return hr;
    DiWrap *w=(DiWrap*)calloc(1,sizeof *w);
    w->lpVtbl=&g_diVtbl; w->real=(IDirectInput8A*)*ppv;
    *ppv=w;
    plog("DirectInput8Create: proxy engaged (left-stick lY de-inverter active)");
    return hr;
}

BOOL WINAPI DllMain(HINSTANCE h, DWORD reason, LPVOID r){
    (void)h;(void)r;
    if(reason==DLL_PROCESS_ATTACH) DisableThreadLibraryCalls(h);
    return TRUE;
}
