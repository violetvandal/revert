// host.cpp — an executable regression test for the chain-aware hook in ../vv_hook.h.
//
// The bug this guards against is silent. VV.HudFix and VV.KeyboardGrid both hook the same address
// (the GetScreenElement-by-id resolver), and a 5-byte jmp is a 5-byte jmp: if the second mod to
// load simply writes its own, it overwrites the first one's, and that first mod's detour is never
// called again. Nothing crashes. Nothing logs. The HUD just quietly stops moving to the top-left,
// and you find out from a screenshot weeks later.
//
// So this test loads two mods that both hook, in BOTH orders, and asserts that BOTH of their
// callbacks run and that the original instructions still execute correctly afterwards.
//
// It works by standing up a fake "game" at the real hook address: VirtualAlloc reserves the page
// containing 0x4aae53 and we write the exact 5 bytes THUG2 has there, followed by a clean return.
// The hook code under test is then completely unmodified — it patches what it thinks is the game.
//
// Build + run: ./build.sh   (32-bit mingw, executed under wine)
#include <windows.h>
#include <cstdint>
#include <cstdio>
#include <cstring>

static int failures = 0;

static void check(bool cond, const char* what) {
    printf("  %s %s\n", cond ? "PASS" : "FAIL", what);
    if (!cond) failures++;
}

// lay_site writes a stand-in for the game's code at the real hook address.
//
//   0x4aae53:  56 8D 4C 24 0C   push esi ; lea ecx,[esp+0xc]   <- the 5 bytes a hook displaces
//   0x4aae58:  5E C3            pop esi  ; ret                 <- stand-in for "the rest of it"
//
// Calling 0x4aae53 is therefore stack-balanced and returns cleanly — so if a detour chain corrupts
// the stack, this test crashes instead of quietly passing.
static void lay_site() {
    uint8_t* site = (uint8_t*)0x004aae53;
    static const uint8_t code[] = {0x56, 0x8D, 0x4C, 0x24, 0x0C, 0x5E, 0xC3};
    DWORD old = 0;
    VirtualProtect(site, sizeof code, PAGE_READWRITE, &old);
    memcpy(site, code, sizeof code);
    VirtualProtect(site, sizeof code, PAGE_EXECUTE_READ, &old);
    FlushInstructionCache(GetCurrentProcess(), site, sizeof code);
}

// call_site invokes the patched function with a sentinel in EAX, the way the game does: the
// resolver has just returned the screen element there, and every detour in the chain reads it.
// If the chain fails to preserve EAX, the second callback sees garbage and the test fails.
static void call_site(uint32_t element) {
    void* site = (void*)0x004aae53;
    __asm__ __volatile__(
        "movl %0, %%eax\n\t"
        "call *%1\n\t"
        :
        : "r"(element), "r"(site)
        : "eax", "ecx", "edx", "memory");
}

int main(int argc, char** argv) {
    const char* order = (argc > 1) ? argv[1] : "ab";

    // Make sure the page the hook site lives on is usable. With the host rebased to 0x10000000
    // (see build.sh), 0x004aa000 falls in wine's already-committed private range, so we don't
    // reserve it — we just confirm it's backed, then lay_site() writes the fake game code with a
    // VirtualProtect, exactly as it would over THUG2.exe's real .text.
    MEMORY_BASIC_INFORMATION mbi;
    if (!VirtualQuery((void*)0x004aae53, &mbi, sizeof mbi) || mbi.State != MEM_COMMIT) {
        // Not backed on this wine build — commit it ourselves. (Harmless if it's already there.)
        if (!VirtualAlloc((void*)0x004aa000, 0x2000, MEM_RESERVE | MEM_COMMIT, PAGE_READWRITE)) {
            printf("FAIL: 0x4aae53 is not backed and could not be committed (err %lu)\n", GetLastError());
            return 1;
        }
    }

    HMODULE a = LoadLibraryA("modA.dll");
    HMODULE b = LoadLibraryA("modB.dll");
    if (!a || !b) { printf("FAIL: could not load the test mods\n"); return 1; }

    auto a_install = (int (*)())GetProcAddress(a, "a_install");
    auto b_install = (int (*)())GetProcAddress(b, "b_install");
    auto a_hits = (volatile long*)GetProcAddress(a, "a_hits");
    auto b_hits = (volatile long*)GetProcAddress(b, "b_hits");
    auto a_last = (volatile uint32_t*)GetProcAddress(a, "a_last");
    auto b_last = (volatile uint32_t*)GetProcAddress(b, "b_last");
    if (!a_install || !b_install || !a_hits || !b_hits) { printf("FAIL: bad test mod exports\n"); return 1; }

    // ── the real test: both mods hook the same address, in the given order ──
    printf("[chain] install order: %s\n", order);
    lay_site();
    int ia, ib;
    if (order[0] == 'b') { ib = b_install(); ia = a_install(); }
    else                 { ia = a_install(); ib = b_install(); }
    check(ia == 1, "mod A installed");
    check(ib == 1, "mod B installed");

    *a_hits = *b_hits = 0;
    *a_last = *b_last = 0;
    call_site(0xDEADBEEF);

    // The point of the whole exercise. Before the chaining fix, exactly one of these was 1 and the
    // other was 0 — whichever mod hooked FIRST got silently unhooked by the second.
    check(*a_hits == 1, "mod A's detour ran (it would be 0 if B clobbered its hook)");
    check(*b_hits == 1, "mod B's detour ran (it would be 0 if A clobbered its hook)");

    // Both must see the SAME element. This is what proves the chain preserves EAX across detours.
    check(*a_last == 0xDEADBEEF, "mod A saw the element in EAX");
    check(*b_last == 0xDEADBEEF, "mod B saw the element in EAX");

    // Reaching here at all means the original `push esi; lea ecx,[esp+0xc]` still executed and the
    // function returned — i.e. no detour left the stack unbalanced.
    check(true, "the original instructions ran and the call returned cleanly");

    // ── the site must still work when called repeatedly ──
    *a_hits = *b_hits = 0;
    for (int i = 0; i < 100; i++) call_site(0x1234);
    check(*a_hits == 100 && *b_hits == 100, "100 calls -> 100 hits in each mod (chain is stable)");

    // ── refuse an exe we were not built against ──
    // A hook that binds to whatever happens to sit at a hardcoded address in an exe it does not
    // recognise is worse than no hook at all, so vv_install_hook checks the bytes first.
    uint8_t* site = (uint8_t*)0x004aae53;
    DWORD old = 0;
    VirtualProtect(site, 5, PAGE_READWRITE, &old);
    memset(site, 0x90, 5);                       // nops: not our 5 bytes, and not an E9 either
    VirtualProtect(site, 5, PAGE_EXECUTE_READ, &old);
    check(a_install() == 0, "install REFUSED on an unrecognised exe (bytes don't match)");

    printf(failures ? "\nFAILED (%d)\n" : "\nOK\n", failures);
    return failures ? 1 : 0;
}
