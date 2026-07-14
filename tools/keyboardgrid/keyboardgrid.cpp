// VV.KeyboardGrid — Violet Vandal Edition: controller text entry for THUG2 PC.
//
// Text-entry screens (create-a-skater name, etc.) are keyboard-only on PC. The game reads typed
// characters via Win32 WM_CHAR, so PostMessage(WM_CHAR, ch) to the game window runs the game's OWN
// input path (append + on-screen redraw) exactly like a physical keypress. We drive that from the
// CONTROLLER (read via DirectInput — the pad is a DInput device, not XInput) or the keyboard.
//
// Controller: D-pad/left-stick cycle the letter, A commits, X backspaces, Start = Enter/save.
// Keyboard F-key entry (F5/F6/F7/F8/F10) stays fully working alongside it.
#define DIRECTINPUT_VERSION 0x0800
#include <windows.h>
#include <dinput.h>
#include <cstdint>
#include <cstring>
#include <cstdio>
#include "../common/vv_hook.h"

static const uint32_t VT_TEXTELEM = 0x0064a290;
static const uint32_t ID_KBCURSTR = 0x6df45f28;
static const int OFF_ID = 0x14;
static const char CHARSET[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789 ";
static const int NCHARS = (int)sizeof CHARSET - 1;

static HINSTANCE g_hinst = 0;
static void lg(const char* fmt, long a = 0, long b = 0) {
    FILE* f = fopen("vv_kbgrid.log", "a"); if (!f) return; fprintf(f, fmt, a, b); fclose(f);
}

// ---- active text field detection ----
// Safe read of our OWN process memory. ReadProcessMemory does the access check in the
// kernel and returns FALSE (instead of faulting the caller) if the range is inaccessible.
// The old scan dereferenced raw pointers straight from VirtualQuery results; THUG2 could
// free / re-protect a region between the query and the read (a TOCTOU race), which Wine
// tolerated but native Windows kills with 0xc0000005. Reading via RPM closes that hole.
static bool safe_read(const void* addr, void* buf, size_t n) {
    SIZE_T got = 0;
    return ReadProcessMemory(GetCurrentProcess(), addr, buf, n, &got) && got == n;
}

// ---- the gate ----
// text_field_active() below sweeps every committed read-write region of the address space. That
// is not cheap, and the old code ran it every ~528ms FOREVER — at the main menu, mid-run, in a
// level, always — burning ~12% of a core to keep re-answering "no, there is still no text field".
//
// So we gate it on VV_HOOK_SITE, the GetScreenElement-by-id resolver: when it hands back the
// keyboard_current_string element, a text field just came up and it is worth sweeping. That makes
// the common case near-free.
//
// ⚠️ BUT this feature already SHIPS and is user-validated (v1.2.6, Deck, 3 screen types), and the
// assumption "keyboard_current_string is resolved through 0x4aae53" is NOT verified — it is the one
// thing this change rests on that a playtest still has to confirm. So the gate is FAIL-SAFE: the
// worker ALSO forces a sweep every few seconds regardless. If the hook fires as expected the gate
// makes activation instant; if it somehow never fires for a given text screen, the forced sweep
// still finds the field within a few seconds. Worst case is a slightly slower cursor, NEVER a dead
// feature. Correctness must not depend on an unproven assumption about a working shipped mod.
//
// The sweep stays the ORACLE either way — the hook (and the forced tick) only decide WHEN to run
// it; only the sweep confirms the element is still live on screen. Getting that wrong is not a perf
// bug, it is the controller typing into the game while you skate.
static volatile LONG g_gate = 0;    // the hook has seen keyboard_current_string since we last idled
static bool g_hooked = false;       // did the hook install? (if not, we sweep every tick as before)

extern "C" void kb_on_resolve(uint32_t* el) {
    if (!el) return;
    if (el[0] == VT_TEXTELEM && el[0x14/4] == ID_KBCURSTR) g_gate = 1;
}
VV_HOOK_DETOUR(kb_detour, kb_on_resolve)

static bool text_field_active() {
    static unsigned char chunk[0x10000];              // reused 64 KB window (no per-scan malloc)
    const size_t STEP = sizeof chunk - (OFF_ID + 4);  // overlap so a match on a chunk boundary isn't missed
    SYSTEM_INFO si; GetSystemInfo(&si);
    unsigned char* addr = (unsigned char*)si.lpMinimumApplicationAddress;
    unsigned char* end  = (unsigned char*)si.lpMaximumApplicationAddress;
    MEMORY_BASIC_INFORMATION mbi;
    while (addr < end && VirtualQuery(addr, &mbi, sizeof mbi)) {
        unsigned char* next = (unsigned char*)mbi.BaseAddress + mbi.RegionSize;
        if (mbi.State == MEM_COMMIT) {
            DWORD pr = mbi.Protect & 0xff;
            bool rw = (pr == PAGE_READWRITE || pr == PAGE_EXECUTE_READWRITE ||
                       pr == PAGE_WRITECOPY || pr == PAGE_EXECUTE_WRITECOPY);
            if (rw && !(mbi.Protect & PAGE_GUARD) && mbi.RegionSize <= (512u << 20)) {
                unsigned char* base = (unsigned char*)mbi.BaseAddress;
                size_t region = mbi.RegionSize;
                for (size_t off = 0; off < region; off += STEP) {
                    size_t want = region - off; if (want > sizeof chunk) want = sizeof chunk;
                    if (!safe_read(base + off, chunk, want)) continue; // raced/freed -> skip, never fault
                    for (size_t i = 0; i + OFF_ID + 4 <= want; i += 4)
                        if (*(uint32_t*)(chunk + i) == VT_TEXTELEM && *(uint32_t*)(chunk + i + OFF_ID) == ID_KBCURSTR) return true;
                }
            }
        }
        if (next <= addr) break; addr = next;
    }
    return false;
}

// ---- game window + WM_CHAR ----
static HWND g_hwnd = 0;
static BOOL CALLBACK enumcb(HWND h, LPARAM) {
    DWORD pid = 0; GetWindowThreadProcessId(h, &pid);
    if (pid == GetCurrentProcessId() && GetWindow(h, GW_OWNER) == 0 && IsWindowVisible(h)) { g_hwnd = h; return FALSE; }
    return TRUE;
}
static HWND game_window() { if (g_hwnd && IsWindow(g_hwnd)) return g_hwnd; g_hwnd = 0; EnumWindows(enumcb, 0); return g_hwnd; }
static void post_char(char c) { HWND h = game_window(); if (h) PostMessageA(h, WM_CHAR, (WPARAM)(unsigned char)c, 0); }
static void post_back() { post_char('\b'); }
// Send Enter (save/confirm) as the keyboard would: keydown + char + keyup, so the game sees it
// however it handles Return (WM_KEYDOWN vs WM_CHAR 0x0D).
static void post_enter() {
    HWND h = game_window(); if (!h) return;
    PostMessageA(h, WM_KEYDOWN, VK_RETURN, 0x001C0001);
    PostMessageA(h, WM_CHAR,    VK_RETURN, 0x001C0001);   // VK_RETURN == 0x0D
    PostMessageA(h, WM_KEYUP,   VK_RETURN, 0xC01C0001);
}

// ---- DirectInput pad ----
static LPDIRECTINPUT8 g_di = 0;
static LPDIRECTINPUTDEVICE8 g_dev = 0;
static BOOL CALLBACK enum_dev(const DIDEVICEINSTANCEA* inst, void*) {
    if (!g_dev && SUCCEEDED(g_di->CreateDevice(inst->guidInstance, &g_dev, NULL))) {
        FILE* f = fopen("vv_kbgrid.log", "a"); if (f) { fprintf(f, "[di] using pad: %s\n", inst->tszProductName); fclose(f); }
        return DIENUM_STOP;
    }
    return DIENUM_CONTINUE;
}
static bool di_init() {
    if (FAILED(DirectInput8Create(g_hinst, DIRECTINPUT_VERSION, IID_IDirectInput8A, (void**)&g_di, NULL))) { lg("[di] create fail\n"); return false; }
    g_di->EnumDevices(DI8DEVCLASS_GAMECTRL, enum_dev, NULL, DIEDFL_ATTACHEDONLY);
    if (!g_dev) { lg("[di] no pad found\n"); return false; }
    g_dev->SetDataFormat(&c_dfDIJoystick2);
    HWND h = game_window();
    g_dev->SetCooperativeLevel(h, DISCL_BACKGROUND | DISCL_NONEXCLUSIVE);
    // normalize the left-stick Y axis to -1000..1000 (center 0) so we can threshold it
    DIPROPRANGE rng; rng.diph.dwSize = sizeof rng; rng.diph.dwHeaderSize = sizeof rng.diph;
    rng.diph.dwObj = DIJOFS_Y; rng.diph.dwHow = DIPH_BYOFFSET; rng.lMin = -1000; rng.lMax = 1000;
    g_dev->SetProperty(DIPROP_RANGE, &rng.diph);
    g_dev->Acquire();
    lg("[di] pad acquired\n");
    return true;
}

// ---- input abstraction ----
enum { IN_TOGGLE, IN_UP, IN_DOWN, IN_COMMIT, IN_BACK, IN_DONE, IN_COUNT };
static bool key_input(int which) {
    switch (which) {
        case IN_TOGGLE: return GetAsyncKeyState(VK_F9) & 0x8000;
        case IN_UP:     return GetAsyncKeyState(VK_F6) & 0x8000;
        case IN_DOWN:   return GetAsyncKeyState(VK_F5) & 0x8000;
        case IN_COMMIT: return GetAsyncKeyState(VK_F7) & 0x8000;
        case IN_BACK:   return GetAsyncKeyState(VK_F8) & 0x8000;
        case IN_DONE:   return GetAsyncKeyState(VK_F10) & 0x8000;
    }
    return false;
}
// Controller mapping (measured on the pad): A=btn0, B=btn1, Select=btn6, Start=btn7; D-Pad = POV[0].
static bool ctrl_input(int which, const DIJOYSTATE2& js) {
    DWORD pov = js.rgdwPOV[0];
    bool povUp   = (pov != 0xFFFFFFFF) && (pov <= 4500 || pov >= 31500);
    bool povDown = (pov != 0xFFFFFFFF) && (pov >= 13500 && pov <= 22500);
    auto b = [&](int n){ return (js.rgbButtons[n] & 0x80) != 0; };
    bool stickUp   = js.lY < -500;   // left stick pushed up   (range set to -1000..1000)
    bool stickDown = js.lY >  500;   // left stick pushed down
    switch (which) {
        case IN_TOGGLE: return b(6);              // Select/Back
        case IN_UP:     return povUp || stickUp;  // D-Pad Up / stick up   -> next letter
        case IN_DOWN:   return povDown || stickDown; // D-Pad Down / stick down -> prev letter
        case IN_COMMIT: return b(0);          // A
        case IN_BACK:   return b(2);          // X  (B is the menu's Cancel, so we avoid it)
        case IN_DONE:   return b(7);          // Start
    }
    return false;
}

static bool g_active = false;
static int  g_cand = 0;
static bool g_shown = false;

static DWORD WINAPI worker(LPVOID) {
    lg("[kbentry] worker enter pid=%lu\n", (long)GetCurrentProcessId());
    Sleep(3000);
    CoInitializeEx(NULL, COINIT_APARTMENTTHREADED);
    di_init();

    bool prev[IN_COUNT] = {0}; int rep[IN_COUNT] = {0};
    int scanctr = 0;

    for (;;) {
        Sleep(33);

        // AUTO-DETECT: enable controller text entry whenever a text field is on screen, disable
        // when it goes away. Throttled (~0.5s) since the field-scan sweeps memory. A short
        // stability filter (present for 2 consecutive scans, ~1s) avoids the transient
        // keyboard_current_string elements that flicker in during menu loading.
        //
        // Decide whether to sweep this tick (~528ms). Sweep if the gate is open (the hook saw a
        // text element), OR the hook never installed (unknown exe → old always-sweep behaviour), OR
        // it has been a few seconds since the last sweep. That last clause is the fail-safe: it
        // bounds how long a text field can be up before we notice it, so the feature keeps working
        // even if the hook assumption is wrong — see the gate comment above.
        static int streak = 0, since = 0;
        if (++scanctr >= 16) {
            scanctr = 0;
            bool forced = (++since >= 6);   // ~every 3.2s (6 * 528ms), independent of the hook
            bool gate = (!g_hooked) || (g_gate != 0) || forced;
            if (gate) {                     // NB: only the SWEEP is gated; the input handling below
                since = 0;                  // always runs, so an active field is never starved
                bool present = text_field_active();
                streak = present ? streak + 1 : 0;
                if (streak >= 2 && !g_active) { g_active = true; g_cand = 0; g_shown = false; lg("[kbentry] AUTO ON\n"); }
                else if (!present) {
                    if (g_active) { g_active = false; g_cand = 0; g_shown = false; lg("[kbentry] AUTO OFF\n"); }
                    g_gate = 0;   // field gone: back to the gated cadence until the hook or the
                                  // forced tick reopens it
                }
            }
        }

        // read DInput pad
        DIJOYSTATE2 js; bool have = false;
        if (g_dev) {
            g_dev->Poll();
            HRESULT hr = g_dev->GetDeviceState(sizeof js, &js);
            if (hr == DIERR_INPUTLOST || hr == DIERR_NOTACQUIRED) { g_dev->Acquire(); }
            else if (SUCCEEDED(hr)) have = true;
        }

        // keyboard F-keys OR controller drive the UX
        bool cur[IN_COUNT], edge[IN_COUNT];
        for (int i = 0; i < IN_COUNT; i++) { cur[i] = key_input(i) || (have && ctrl_input(i, js)); edge[i] = cur[i] && !prev[i]; prev[i] = cur[i]; }
        for (int i = IN_UP; i <= IN_DOWN; i++) { if (cur[i]) { if (++rep[i] > 9) { rep[i] = 6; edge[i] = true; } } else rep[i] = 0; }

        if (!g_active) continue;
        if (edge[IN_UP] || edge[IN_DOWN]) {
            if (g_shown) { post_back(); g_cand = edge[IN_UP] ? (g_cand + 1) % NCHARS : (g_cand - 1 + NCHARS) % NCHARS; }
            post_char(CHARSET[g_cand]); g_shown = true;
        }
        if (edge[IN_COMMIT]) { g_shown = false; g_cand = 0; }
        if (edge[IN_BACK])   { post_back(); g_shown = false; g_cand = 0; }
        if (edge[IN_DONE])   { post_enter(); g_active = false; g_shown = false; g_cand = 0; lg("[kbentry] DONE (enter)\n"); }
    }
    return 0;
}

BOOL APIENTRY DllMain(HMODULE h, DWORD r, LPVOID) {
    if (r != DLL_PROCESS_ATTACH) return TRUE;
    g_hinst = (HINSTANCE)h;
    DisableThreadLibraryCalls(h);
    lg("[kbentry] loaded pid=%lu\n", (long)GetCurrentProcessId());
    // Cold, before any game thread runs. Chains automatically if VV.HudFix already hooked the
    // same site (or gets chained onto if it loads after us) — see ../common/vv_hook.h.
    g_hooked = vv_install_hook((void*)&kb_detour);
    lg(g_hooked ? "[kbentry] hook installed (scan gated on it)\n"
                : "[kbentry] hook NOT installed — falling back to always scanning\n");
    CreateThread(0, 0, worker, 0, 0, 0);
    return TRUE;
}
