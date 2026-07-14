// VV.HudFix — Violet Vandal Edition HUD fix (THUG2 PC, no-CD exe md5 d464781a...).
//
// FixHUD (ThirteenAG WidescreenFix) pillarbox-centers the whole HUD via one global X-offset
// (0x786d88 = (width-height*4/3)/2). This pulls just our HUD targets back to the true left
// edge, resolution-independently. Hook: the GetScreenElement-by-id resolver right after it
// returns a found element in EAX (0x4aae53). Element stores flat id @ +0x14, position @ +0x90
// (copies +0x9c/+0xd4/+0xe0). Target X is rewritten to (scriptX - offset/scale).
//
//  * the_score (0x675adbaa): static element -> one-shot rewrite when X still == raw (136).
//    Its star/SPECIAL-bar children are parented to it and ride along.
//  * goal_points_display (0xfa426fbe, "X / Y GOAL PTS." container): ANIMATED via
//    DoScreenElementMorph (slides to a center-relative rest pos after each goal update), so a
//    one-shot write loses to the animation. We capture its pointer in the hook and a worker
//    thread continuously pins its X = -offset/scale (raw X is 0 = canvas left), overriding the
//    morph so it stays top-left. (Side effect: its slide-in is suppressed; alpha fade is kept.)
//
// Runs alongside the stock WidescreenFix. No-ops if FixHUD is off (offset==0).
//
// The hook itself lives in ../common/vv_hook.h, because VV.KeyboardGrid hooks the SAME address
// and a second raw 5-byte jmp here would silently overwrite whichever of us loaded first. See
// that header — the chaining is the whole point of it.
#include <windows.h>
#include <cstdint>
#include <cstring>
#include "../common/vv_hook.h"

static int32_t* const HUD_OFFSET = (int32_t*)0x00786d88;
static float*   const HUD_SCALE  = (float*)  0x00786d80;
static const uint32_t ID_SCORE = 0x675adbaa;
static const uint32_t ID_GPTS  = 0xfa426fbe;
static const int XO[4] = {0x90, 0x9c, 0xd4, 0xe0};

extern "C" volatile uint32_t* g_gpd = nullptr;   // goal_points_display element (animated)

// readable(p,n) — is p..p+n committed and readable?
//
// NOT IsBadReadPtr. IsBadReadPtr validates by faulting and catching, and on Apple Silicon every
// fault is a Mach round-trip through Rosetta's exception path. The worker below runs at 250 Hz,
// so the old code drove that fault path a thousand times a second — and the crash log shows this
// thread taking an access violation inside the probe and unwinding into "Exception frame is not
// in stack limits". VirtualQuery asks the kernel the same question without ever faulting.
static bool readable(const void* p, size_t n) {
    MEMORY_BASIC_INFORMATION mbi;
    if (!VirtualQuery(p, &mbi, sizeof mbi)) return false;
    if (mbi.State != MEM_COMMIT) return false;
    if (mbi.Protect & (PAGE_NOACCESS | PAGE_GUARD)) return false;
    const uint8_t* base = (const uint8_t*)mbi.BaseAddress;
    return (const uint8_t*)p + n <= base + mbi.RegionSize;   // no straddle into the next region
}

static bool hud_voff(float* out) {
    if (!readable(HUD_OFFSET, 4) || !readable(HUD_SCALE, 4)) return false;
    float scale = *HUD_SCALE; int32_t off = *HUD_OFFSET;
    if (scale <= 0.01f || off == 0) return false;
    *out = (float)off / scale; return true;
}

extern "C" void hud_on_resolve(uint32_t* el) {
    if (!el) return;
    uint32_t id = el[0x14/4];
    if (id == ID_SCORE) {
        float voff; if (!hud_voff(&voff)) return;
        for (int i=0;i<4;i++){ float* x=(float*)((uint8_t*)el+XO[i]); if (*x==136.0f) *x=136.0f-voff; }
    } else if (id == ID_GPTS) {
        g_gpd = el;                                   // remember it; worker thread pins it
    }
}

VV_HOOK_DETOUR(hud_detour, hud_on_resolve)           // hook body at 0x4aae53 (eax = element)

static DWORD WINAPI worker(LPVOID) {
    // Pin the animated goal-points container to the left. No code patching here any more.
    for (;;) {
        Sleep(4);
        uint32_t* el = (uint32_t*)g_gpd;
        if (!el || !readable(el, 0x100)) continue;
        if (el[0x14/4] != ID_GPTS) { g_gpd = nullptr; continue; }   // freed/reused -> drop
        float voff; if (!hud_voff(&voff)) continue;
        float target = 0.0f - voff;                                  // raw X is 0 (canvas left)
        for (int i=0;i<4;i++){ float* x=(float*)((uint8_t*)el+XO[i]); if (*x != target) *x = target; }
    }
    return 0;
}

BOOL APIENTRY DllMain(HMODULE h, DWORD r, LPVOID) {
    if (r == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(h);
        vv_install_hook((void*)&hud_detour);   // cold, before any game thread runs; chains if
                                               // VV.KeyboardGrid already hooked the same site
        CreateThread(0,0,worker,0,0,0);        // pinning only
    }
    return TRUE;
}
