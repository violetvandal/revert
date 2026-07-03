// VV.GlyphFix — Violet Vandal Edition: selectable button-prompt glyphs for THUG2 PC.
//
// THUG2's trick-combo prompts are font glyph tokens (goal_tetris_trick_text, e.g. "\b4 + \b2").
// The PC font renderer routes the four FACE-button glyph slots (0-3) to the keyboard-key path
// (shows "kp2"); 4-7 (d-pad) draw as glyphs. Two levers, both per glyph style:
//   * RENDERER: NOP the `cmp al,4 / jb <keyname>` at 0x4ced6f & 0x4cff38 -> face buttons draw as
//     glyphs. Toggleable live (restore 72 0c to go back to keyboard "kp2").
//   * FONT: the buttons-font name immediate (ButtonsXbox) lives at 0x48d983; repoint it to
//     "ButtonsPs2"/"ButtonsNgc" (ship in fonts.prx) to theme the glyphs. Must be set before the
//     font loads at startup -> done in DllMain. (Can't change live; applies on next launch.)
//
// STYLE comes from two sources, in priority order:
//   1. In-game MOD OPTIONS "Button Glyphs" menu -> 3 GlobalFlags (SET/B0/B1). The .asi reads the
//      flag bitfield directly (flagmgr = *(*0x7ce478+0x20); bits at flagmgr+0x5f0) and, while the
//      game runs, applies the keyboard<->controller toggle LIVE and writes vv_glyphs.cfg so the
//      chosen font theme applies on the NEXT launch.
//   2. vv_glyphs.cfg (what the menu last wrote) > VV_GLYPHS env (launcher default; Steam Deck auto)
//      > xbox. Read in DllMain to pick the boot font + initial renderer state.
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

// ---- patch helpers ----------------------------------------------------------
static void patch_dword(uint32_t va, uint32_t val) {
    uint8_t* p = (uint8_t*)(uintptr_t)va; DWORD o;
    VirtualProtect(p, 4, PAGE_EXECUTE_READWRITE, &o);
    *(volatile uint32_t*)p = val;
    VirtualProtect(p, 4, o, &o);
}
static void set_branch(uint32_t va, bool nop) {     // nop=glyphs, restore=keyboard
    uint8_t* p = (uint8_t*)(uintptr_t)va; DWORD o;
    VirtualProtect(p, 2, PAGE_EXECUTE_READWRITE, &o);
    if (nop) { p[0] = 0x90; p[1] = 0x90; } else { p[0] = 0x72; p[1] = 0x0c; }
    VirtualProtect(p, 2, o, &o);
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
static bool get_flag(int idx, bool* val) {
    uint32_t root = *(volatile uint32_t*)(uintptr_t)FLAG_ROOT;
    if (root < 0x10000 || root >= 0x80000000 || IsBadReadPtr((void*)(uintptr_t)root, 0x24)) return false;
    uint32_t mgr = *(volatile uint32_t*)(uintptr_t)(root + 0x20);
    uint32_t addr = mgr + 0x5f0 + (idx / 32) * 4;
    if (mgr < 0x10000 || mgr >= 0x80000000 || IsBadReadPtr((void*)(uintptr_t)addr, 4)) return false;
    *val = (*(volatile uint32_t*)(uintptr_t)addr >> (idx % 32)) & 1u;
    return true;
}

static DWORD WINAPI worker(LPVOID) {
    Sleep(6000);                                   // combo text renders in menus — let the game settle
    apply_renderer(g_boot_style != ST_KEYBOARD);   // initial state from the boot style

    int last = -2;
    for (;;) {
        Sleep(1000);
        bool set = false;
        if (!get_flag(F_SET, &set)) { apply_renderer(g_boot_style != ST_KEYBOARD); continue; }  // flags not ready
        int desired;
        if (!set) {
            desired = g_env_style;                 // Default -> defer to the launcher default
        } else {
            bool b0 = false, b1 = false; get_flag(F_B0, &b0); get_flag(F_B1, &b1);
            desired = (b0 ? 1 : 0) + (b1 ? 2 : 0); // 0=kb 1=xbox 2=ps 3=gc
        }
        if (desired == last) continue;
        apply_renderer(desired != ST_KEYBOARD);    // keyboard<->controller is live
        if (set) write_cfg(desired); else delete_cfg();   // font theme applies on next launch
        last = desired;
    }
    return 0;
}

BOOL APIENTRY DllMain(HMODULE h, DWORD r, LPVOID) {
    if (r != DLL_PROCESS_ATTACH) return TRUE;
    DisableThreadLibraryCalls(h);

    const char* e = getenv("VV_GLYPHS");
    g_env_style = e ? parse_style(e) : ST_XBOX;

    int cfg;
    g_boot_style = read_cfg(&cfg) ? cfg : g_env_style;   // in-game choice wins over launcher default
    apply_font(g_boot_style);                            // must happen before startup loads the font

    CreateThread(0, 0, worker, 0, 0, 0);
    return TRUE;
}
