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
#include <windows.h>
#include <cstdint>

static int32_t* const HUD_OFFSET = (int32_t*)0x00786d88;
static float*   const HUD_SCALE  = (float*)  0x00786d80;
static const uint32_t ID_SCORE = 0x675adbaa;
static const uint32_t ID_GPTS  = 0xfa426fbe;
static const int XO[4] = {0x90, 0x9c, 0xd4, 0xe0};

extern "C" volatile uint32_t* g_gpd = nullptr;   // goal_points_display element (animated)

static bool hud_voff(float* out) {
    if (IsBadReadPtr((void*)HUD_OFFSET,4) || IsBadReadPtr((void*)HUD_SCALE,4)) return false;
    float scale = *HUD_SCALE; int32_t off = *HUD_OFFSET;
    if (scale <= 0.01f || off == 0) return false;
    *out = (float)off / scale; return true;
}

extern "C" void on_resolve(uint32_t* el) {
    uint32_t id = el[0x14/4];
    if (id == ID_SCORE) {
        float voff; if (!hud_voff(&voff)) return;
        for (int i=0;i<4;i++){ float* x=(float*)((uint8_t*)el+XO[i]); if (*x==136.0f) *x=136.0f-voff; }
    } else if (id == ID_GPTS) {
        g_gpd = el;                                   // remember it; worker thread pins it
    }
}

__attribute__((naked)) static void detour() {        // hook body at 0x4aae53 (eax = element)
    asm volatile(
        "pusha\n\t" "pushf\n\t"
        "push %eax\n\t" "call _on_resolve\n\t" "add $4,%esp\n\t"
        "popf\n\t" "popa\n\t"
        "push %esi\n\t" "lea 0xc(%esp),%ecx\n\t"
        "push $0x004aae58\n\t" "ret\n\t");
}

static DWORD WINAPI worker(LPVOID) {
    Sleep(6000);
    uint8_t* hook = (uint8_t*)0x004aae53; DWORD old;
    VirtualProtect(hook, 5, PAGE_EXECUTE_READWRITE, &old);
    hook[0] = 0xE9; *(int32_t*)(hook+1) = (int32_t)&detour - (int32_t)(hook+5);
    VirtualProtect(hook, 5, old, &old);
    // continuously pin the animated goal-points container to the left
    for (;;) {
        Sleep(4);
        volatile uint32_t* el = g_gpd;
        if (!el || IsBadReadPtr((void*)el, 0x100)) continue;
        if (el[0x14/4] != ID_GPTS) { g_gpd = nullptr; continue; }   // freed/reused -> drop
        float voff; if (!hud_voff(&voff)) continue;
        float target = 0.0f - voff;                                  // raw X is 0 (canvas left)
        for (int i=0;i<4;i++){ float* x=(float*)((uint8_t*)el+XO[i]); if (*x != target) *x = target; }
    }
    return 0;
}

BOOL APIENTRY DllMain(HMODULE h, DWORD r, LPVOID) {
    if (r == DLL_PROCESS_ATTACH) { DisableThreadLibraryCalls(h); CreateThread(0,0,worker,0,0,0); }
    return TRUE;
}
