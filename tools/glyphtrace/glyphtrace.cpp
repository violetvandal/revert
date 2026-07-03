// VV.GlyphTrace — read-only diagnostic for the THUG2 PC trick-combo display ("kp2") problem.
// no-CD THUG2.exe (md5 d464781a...), base 0x400000, no ASLR.
//
// Wraps two trick-display cfuncs (cdecl, arg0 = script params struct at [esp+4]):
//   GetTrickDisplayText            @ 0x5b1250
//   GetKeyComboArrayFromTrickArray @ 0x5b1890
// Each wrapper runs the ORIGINAL via a trampoline, then deep-scans the params struct
// (following pointers 2 levels deep) and logs every ASCII string it finds, with a hex
// dump of the first bytes — so we can see whether the on-screen combo is rendered as
// keyboard key-names ("kp2") or as font glyph tokens (backslash 0x5c + index).
//
// Output: vv_glyph_trace.log in the game's working directory (where THUG2.exe runs).
// Deduped (by FNV hash) and capped at 400 unique strings so it can't run away.
#include <windows.h>
#include <cstdint>
#include <cstdio>
#include <cstring>

static FILE* g_log = nullptr;
static CRITICAL_SECTION g_cs;
static uint32_t g_seen[1024];
static int g_nseen = 0;
static int g_total = 0;

static uint32_t fnv(const char* s) {
    uint32_t h = 2166136261u;
    while (*s) { h = (h ^ (uint8_t)*s++) * 16777619u; }
    return h;
}
static bool seen(uint32_t h) {
    for (int i = 0; i < g_nseen; i++) if (g_seen[i] == h) return true;
    if (g_nseen < 1024) g_seen[g_nseen++] = h;
    return false;
}

// Is p a printable ASCII C-string of length 2..63? (backslash 0x5c allowed = glyph token)
static bool is_str(const char* p, int* outlen) {
    if (IsBadReadPtr(p, 2)) return false;
    int n = 0;
    for (; n < 64; n++) {
        if (IsBadReadPtr(p + n, 1)) return false;
        char c = p[n];
        if (c == 0) break;
        unsigned char u = (unsigned char)c;
        if (u < 0x20 || u > 0x7e) return false;
    }
    if (n < 2 || n >= 64) return false;
    *outlen = n;
    return true;
}

static void logstr(const char* tag, const char* s) {
    uint32_t h = fnv(s);
    if (seen(h)) return;
    if (g_total++ > 400) return;
    if (!g_log) return;
    fprintf(g_log, "[%s] \"%s\"  | hex:", tag, s);
    for (int i = 0; s[i] && i < 28; i++) fprintf(g_log, " %02x", (unsigned char)s[i]);
    fprintf(g_log, "\n");
    fflush(g_log);
}

// Walk struct memory; log strings; follow plausible pointers up to `depth` more levels.
static int g_budget;
static void scan(const char* tag, uint8_t* p, int depth) {
    if (!p || depth < 0 || g_budget <= 0 || IsBadReadPtr(p, 4)) return;
    for (int off = 0; off < 0x80; off += 4) {
        if (--g_budget <= 0) return;
        if (IsBadReadPtr(p + off, 4)) break;
        uint32_t d = *(uint32_t*)(p + off);
        if (d < 0x00010000 || d >= 0x80000000) continue;
        int len;
        if (is_str((const char*)(uintptr_t)d, &len)) {
            logstr(tag, (const char*)(uintptr_t)d);
        } else if (depth > 0) {
            scan(tag, (uint8_t*)(uintptr_t)d, depth - 1);
        }
    }
}

typedef int (__cdecl *fn_t)(void*);
static fn_t orig_gtdt = nullptr;
static fn_t orig_gkc  = nullptr;

static void dump(const char* tag, void* params, int ret) {
    if (g_total > 400) return;
    EnterCriticalSection(&g_cs);
    if (g_log) { fprintf(g_log, "--- %s ret=%d params=%p ---\n", tag, ret, params); fflush(g_log); }
    g_budget = 4000;
    scan(tag, (uint8_t*)params, 2);
    LeaveCriticalSection(&g_cs);
}

extern "C" int __cdecl hook_gtdt(void* params) {
    int r = orig_gtdt(params);
    dump("GetTrickDisplayText", params, r);
    return r;
}
extern "C" int __cdecl hook_gkc(void* params) {
    int r = orig_gkc(params);
    dump("GetKeyComboArrayFromTrickArray", params, r);
    return r;
}

static fn_t make_tramp(uint32_t target, int n) {
    uint8_t* t = (uint8_t*)VirtualAlloc(0, 32, MEM_COMMIT | MEM_RESERVE, PAGE_EXECUTE_READWRITE);
    memcpy(t, (void*)(uintptr_t)target, n);
    t[n] = 0xE9;
    *(int32_t*)(t + n + 1) = (int32_t)(target + n) - (int32_t)(uintptr_t)(t + n + 5);
    return (fn_t)t;
}
static void install(uint32_t target, void* hook) {
    DWORD old;
    VirtualProtect((void*)(uintptr_t)target, 5, PAGE_EXECUTE_READWRITE, &old);
    *(uint8_t*)(uintptr_t)target = 0xE9;
    *(int32_t*)(uintptr_t)(target + 1) = (int32_t)(uintptr_t)hook - (int32_t)(target + 5);
    VirtualProtect((void*)(uintptr_t)target, 5, old, &old);
}

static DWORD WINAPI worker(LPVOID) {
    Sleep(8000);                                  // let the game finish init
    g_log = fopen("vv_glyph_trace.log", "w");
    if (g_log) { fprintf(g_log, "VV.GlyphTrace attached. Open Edit Tricks / view a trick's combo.\n"); fflush(g_log); }
    orig_gtdt = make_tramp(0x005b1250, 7);
    orig_gkc  = make_tramp(0x005b1890, 7);
    install(0x005b1250, (void*)hook_gtdt);
    install(0x005b1890, (void*)hook_gkc);
    if (g_log) { fprintf(g_log, "hooks installed.\n"); fflush(g_log); }
    return 0;
}

BOOL APIENTRY DllMain(HMODULE h, DWORD r, LPVOID) {
    if (r == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(h);
        InitializeCriticalSection(&g_cs);
        CreateThread(0, 0, worker, 0, 0, 0);
    }
    return TRUE;
}
