// VV.GlyphFix — Violet Vandal Edition: selectable button-prompt glyphs for THUG2 PC.
//
// THUG2's trick-combo prompts are font glyph tokens (goal_tetris_trick_text, e.g. "\b4 + \b2").
// The PC font renderer routes the four FACE-button glyph slots (0-3) to the keyboard-key path
// (shows "kp2"); 4-7 (d-pad) draw as glyphs. Two levers, both per glyph style:
//   * RENDERER: NOP the `cmp al,4 / jb <keyname>` at 0x4ced6f & 0x4cff38 -> face buttons draw as
//     glyphs. Applied COLD in DllMain so the prompts are correct from the first frame.
//   * FONT: the buttons-font name immediate (ButtonsXbox) lives at 0x48d983; repoint it to
//     "ButtonsPs2"/"ButtonsNgc" (ship in fonts.prx) to theme the glyphs. Must be set before the
//     font loads at startup -> done in DllMain. (Can't change live; applies on next launch.)
//
// STYLE comes from two sources, in priority order:
//   1. In-game MOD OPTIONS "Button Glyphs" menu -> 3 GlobalFlags (SET/B0/B1). The .asi reads the
//      flag bitfield directly (flagmgr = *(*0x7ce478+0x20); bits at flagmgr+0x5f0) and writes
//      vv_glyphs.cfg so the chosen style applies on the NEXT launch.
//   2. vv_glyphs.cfg (what the menu last wrote) > VV_GLYPHS env (launcher default; Steam Deck auto)
//      > xbox. Read in DllMain to pick the boot font + initial renderer state.
//
// LIVE vs COLD (VV_GLYPH_LIVE): the keyboard<->controller renderer toggle can either apply LIVE
// (re-patch code while the game runs) or COLD (only the boot state; menu changes take effect on the
// next launch, like the font theme already does). Live re-patching rewrites instructions the render
// thread is executing, which is a torn-write race — benign-ish on Windows/Linux, but under Rosetta
// on macOS it reliably crashes/freezes the game (see tools/hudfix for the same class of bug). So the
// macOS lane launches with VV_GLYPH_LIVE=0 (cold only); Linux/Windows leave it unset and keep the
// live toggle. Either way the boot state is applied cold, so prompts are right from frame one.
//
// Reversible by removing this .asi. Runs alongside WidescreenFix + VV.HudFix.
#include <windows.h>
#include <cstdint>
#include <cstring>
#include <cstdlib>
#include <cstdio>

enum { ST_KEYBOARD = 0, ST_XBOX = 1, ST_PS = 2, ST_GC = 3 };

static char NAME_PS2[] = "ButtonsPs2";
static char NAME_NGC[] = "ButtonsNgc";

static const uint32_t FONT_IMM = 0x0048d983;   // imm32 of `mov [esp+0xc],0x648afc` (ButtonsXbox)
static const uint32_t BR1 = 0x004ced6f, BR2 = 0x004cff38;   // cmp al,4 / jb <keyname>  (72 0c)
static const uint32_t FLAG_ROOT = 0x007ce478;  // *(*0x7ce478 + 0x20) = flagmgr; bitfield at +0x5f0
// GlobalFlag indices — MUST match mods/.../global_flags (MOD_GLYPH_SET/B0/B1).
static const int F_SET = 387, F_B0 = 388, F_B1 = 389;

static int  g_env_style  = ST_XBOX;   // launcher default (VV_GLYPHS); used when menu = Default
static int  g_boot_style = ST_XBOX;   // resolved at boot (cfg > env)
static int  g_renderer   = -1;        // applied renderer: -1 unknown, 0 keyboard, 1 controller
static bool g_live       = true;      // VV_GLYPH_LIVE: re-patch code live on a menu change (off on macOS)

// ---- patch helpers ----------------------------------------------------------
//
// patch_mem writes into the game's own CODE, which makes it the one part of this mod that is
// sensitive to how the host enforces memory protection.
//
// ⚠️ Never ask for PAGE_EXECUTE_READWRITE. It works on Windows and on Wine/Linux, but macOS
// enforces W^X: a page may not be writable and executable at the SAME time (that is what the
// JIT entitlement exists for), and Rosetta additionally caches its translation of any x86
// code it has already executed. So on Wine/macOS the RWX request FAILS -- and the old code
// ignored the return value, wrote to a still-read-only page, and the game died instantly with
// an access violation.
//
// Do it the way every host accepts: flip to plain READWRITE (no execute), memcpy the bytes in one
// store, restore the original protection, then flush the instruction cache so the CPU -- and
// Rosetta's translation cache -- pick up the new bytes. Bail out if the unprotect fails rather than
// writing into a page we do not own; a missing glyph is not worth a crash.
static bool patch_mem(uint32_t va, const void* bytes, size_t n) {
    void* p = (void*)(uintptr_t)va;
    DWORD old = 0;
    if (!VirtualProtect(p, n, PAGE_READWRITE, &old)) return false;
    memcpy(p, bytes, n);
    DWORD tmp = 0;
    VirtualProtect(p, n, old, &tmp);
    FlushInstructionCache(GetCurrentProcess(), p, n);
    return true;
}
static void patch_dword(uint32_t va, uint32_t val) {
    patch_mem(va, &val, 4);
}
static void set_branch(uint32_t va, bool nop) {     // nop=glyphs, restore=keyboard
    const uint8_t glyphs[2]   = { 0x90, 0x90 };     // nop nop
    const uint8_t keyboard[2] = { 0x72, 0x0c };     // jb <keyname>
    patch_mem(va, nop ? glyphs : keyboard, 2);
}
static void apply_renderer(bool controller) {
    if (g_renderer == (int)controller) return;
    set_branch(BR1, controller); set_branch(BR2, controller);
    g_renderer = controller;
}
static void apply_font(int st) {                    // DllMain only (before the font loads)
    if (st == ST_PS) patch_dword(FONT_IMM, (uint32_t)(uintptr_t)NAME_PS2);
    else if (st == ST_GC) patch_dword(FONT_IMM, (uint32_t)(uintptr_t)NAME_NGC);
    // xbox / keyboard -> leave ButtonsXbox
}

// ---- style <-> string -------------------------------------------------------
static int parse_style(const char* s) {
    char b[32] = {0};
    for (int i = 0; s && s[i] && i < 31; i++) { char c = s[i]; b[i] = (c >= 'A' && c <= 'Z') ? c + 32 : c; }
    if (!strcmp(b, "keyboard")) return ST_KEYBOARD;
    if (!strcmp(b, "playstation") || !strcmp(b, "ps") || !strcmp(b, "ps2")) return ST_PS;
    if (!strcmp(b, "gamecube") || !strcmp(b, "gc") || !strcmp(b, "ngc")) return ST_GC;
    return ST_XBOX;
}
static const char* style_name(int st) {
    return st == ST_KEYBOARD ? "keyboard" : st == ST_PS ? "playstation" : st == ST_GC ? "gamecube" : "xbox";
}

// ---- persistence (vv_glyphs.cfg in the game dir) ----------------------------
static bool read_cfg(int* out) {
    FILE* f = fopen("vv_glyphs.cfg", "r"); if (!f) return false;
    char b[32] = {0}; bool ok = fgets(b, sizeof b, f) != nullptr; fclose(f);
    if (!ok) return false;
    for (int i = 0; b[i]; i++) if (b[i] == '\n' || b[i] == '\r') { b[i] = 0; break; }
    if (!b[0] || !strcmp(b, "default")) return false;
    *out = parse_style(b); return true;
}
static void write_cfg(int st) { FILE* f = fopen("vv_glyphs.cfg", "w"); if (f) { fputs(style_name(st), f); fclose(f); } }
static void delete_cfg() { remove("vv_glyphs.cfg"); }

// ---- read a GlobalFlag straight from the bitfield (no engine call) ----------
//
// readable() rather than IsBadReadPtr: IsBadReadPtr validates by faulting and catching, and on
// Apple Silicon every fault is a Mach round-trip through Rosetta's exception path. VirtualQuery
// asks the kernel the same question and never faults.
static bool readable(const void* p, size_t n) {
    MEMORY_BASIC_INFORMATION mbi;
    if (!VirtualQuery(p, &mbi, sizeof mbi)) return false;
    if (mbi.State != MEM_COMMIT) return false;
    if (mbi.Protect & (PAGE_NOACCESS | PAGE_GUARD)) return false;
    const uint8_t* base = (const uint8_t*)mbi.BaseAddress;
    return (const uint8_t*)p + n <= base + mbi.RegionSize;
}

static bool get_flag(int idx, bool* val) {
    uint32_t root = *(volatile uint32_t*)(uintptr_t)FLAG_ROOT;
    if (root < 0x10000 || root >= 0x80000000 || !readable((void*)(uintptr_t)root, 0x24)) return false;
    uint32_t mgr = *(volatile uint32_t*)(uintptr_t)(root + 0x20);
    uint32_t addr = mgr + 0x5f0 + (idx / 32) * 4;
    if (mgr < 0x10000 || mgr >= 0x80000000 || !readable((void*)(uintptr_t)addr, 4)) return false;
    *val = (*(volatile uint32_t*)(uintptr_t)addr >> (idx % 32)) & 1u;
    return true;
}

// The menu-watcher thread. It always persists the player's choice to vv_glyphs.cfg (font theme,
// and — when live is off — the keyboard<->controller state too), so the pick survives to the next
// launch. It ALSO applies the renderer live, but ONLY when g_live is set.
//
// ⚠️ Live means re-patching the branch bytes at 0x4ced6f / 0x4cff38 while the render thread is
// executing them. That is a torn write into hot code: on Windows/Linux it is tolerable (and it is
// the long-shipped behaviour), but under Rosetta on macOS the main thread reads the branch
// mid-write, takes an access violation, and the game crashes or freezes at the menu. So macOS runs
// with g_live=false (VV_GLYPH_LIVE=0): the boot state is applied COLD in DllMain and a menu change
// takes effect on the next launch — exactly the rule the font theme already follows.
static DWORD WINAPI worker(LPVOID) {
    if (g_live) {
        Sleep(6000);                                   // combo text renders in menus — let the game settle
        apply_renderer(g_boot_style != ST_KEYBOARD);   // initial state from the boot style
    }
    int last = -2;
    for (;;) {
        Sleep(1000);
        bool set = false;
        if (!get_flag(F_SET, &set)) {                  // flags not ready yet
            if (g_live) apply_renderer(g_boot_style != ST_KEYBOARD);
            continue;
        }
        int desired;
        if (!set) {
            desired = g_env_style;                 // Default -> defer to the launcher default
        } else {
            bool b0 = false, b1 = false; get_flag(F_B0, &b0); get_flag(F_B1, &b1);
            desired = (b0 ? 1 : 0) + (b1 ? 2 : 0); // 0=kb 1=xbox 2=ps 3=gc
        }
        if (desired == last) continue;
        if (g_live) apply_renderer(desired != ST_KEYBOARD);   // keyboard<->controller live (native only)
        if (set) write_cfg(desired); else delete_cfg();       // theme (and cold kb toggle) apply next launch
        last = desired;
    }
    return 0;
}

BOOL APIENTRY DllMain(HMODULE h, DWORD r, LPVOID) {
    if (r != DLL_PROCESS_ATTACH) return TRUE;
    DisableThreadLibraryCalls(h);

    const char* e = getenv("VV_GLYPHS");
    g_env_style = e ? parse_style(e) : ST_XBOX;

    const char* live = getenv("VV_GLYPH_LIVE");
    g_live = !(live && live[0] == '0' && live[1] == '\0');   // VV_GLYPH_LIVE=0 -> cold only (macOS)

    int cfg;
    g_boot_style = read_cfg(&cfg) ? cfg : g_env_style;   // in-game choice wins over launcher default
    apply_font(g_boot_style);                            // must happen before startup loads the font
    if (!g_live) apply_renderer(g_boot_style != ST_KEYBOARD);   // COLD — no game thread exists yet

    CreateThread(0, 0, worker, 0, 0, 0);                 // menu watcher (+ live renderer when g_live)
    return TRUE;
}
