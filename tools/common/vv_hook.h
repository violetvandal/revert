// vv_hook.h — the shared, chain-aware code hook for the Violet Vandal .asi mods.
//
// Every VV mod that needs to watch THUG2's screen elements wants the same function: the
// GetScreenElement-by-id resolver, hooked right after it returns the element in EAX (0x4aae53).
// The element carries its flat id at +0x14, so one hook tells you every element the game looks
// up, by name, for free. VV.HudFix and VV.KeyboardGrid both want that. Hence this file.
//
// ⚠️ THE COLLISION THIS EXISTS TO PREVENT.
//
// A hook is a 5-byte `jmp rel32` written over the target. If two mods each write one at the same
// address, the second SILENTLY overwrites the first, and the first mod's detour is simply never
// called again. Nothing crashes, nothing logs, the mod just quietly stops working. We do not
// control the ASI loader's load order, so "whoever lands second wins" is not survivable.
//
// So installing is chain-aware. We read the site before we write it:
//
//   * Virgin site (`push esi; lea ecx,[esp+0xc]`) — we are first. Our detour resumes into
//     vv_hook_orig_tail, which re-executes those two instructions and jumps back to 0x4aae58.
//
//   * Already `E9 rel32` — another VV mod got here first. Decode its detour (site + 5 + rel32)
//     and resume into IT instead. Its detour ends by resuming into whatever IT found, so the
//     chain unwinds correctly however many of us are loaded, in any order.
//
// Note the second case must NOT copy the displaced bytes anywhere: rel32 is relative to the
// instruction's own address, so a byte-for-byte copy of an E9 at a new address points somewhere
// else entirely. Decode it, don't move it.
//
// ⚠️ NO TRAMPOLINE, DELIBERATELY.
//
// The textbook detour copies the displaced bytes into freshly-allocated executable memory. We do
// not, because this runs under Rosetta on macOS, where runtime-generated executable code is
// exactly the class of thing that has already cost this project a session. vv_hook_orig_tail is
// ordinary static code compiled into the DLL — Rosetta translated it at load, like everything
// else. The only executable byte this file ever writes is the 5-byte jmp itself.
//
// ⚠️ COLD ONLY. Call vv_install_hook from DllMain, never from a worker thread.
//
// Patching a hot function while the main thread is inside it is a torn write: the store lands as
// an opcode and then a rel32, and a thread entering between the two reads 0xE9 followed by the
// ORIGINAL trailing bytes and jumps to a garbage offset. That is precisely what froze THUG2 under
// Rosetta when these mods patched from a `Sleep(6000)` worker. In DllMain no game thread exists
// yet, so there is nothing to race.
#pragma once
#include <windows.h>
#include <cstdint>
#include <cstring>

// The hook site: GetScreenElement-by-id, immediately after it returns the element in EAX.
// Addresses are against the no-CD THUG2.exe (md5 d464781a...); `revert doctor` verifies it.
#define VV_HOOK_SITE   0x004aae53u
#define VV_HOOK_RESUME 0x004aae58u   // VV_HOOK_SITE + 5

// The 5 bytes we displace: 56 8D 4C 24 0C = `push esi ; lea ecx,[esp+0xc]`.
#define VV_HOOK_ORIG_BYTES {0x56, 0x8D, 0x4C, 0x24, 0x0C}

// Where our detour resumes: the next mod's detour if one beat us here, else vv_hook_orig_tail.
// Read only from asm, so it must be a real emitted symbol — not static, not optimized away.
extern "C" { void* vv_hook_chain = nullptr; }

// vv_hook_orig_tail re-executes the displaced instructions and jumps back past our patch.
// Used only when we are the FIRST mod to hook the site.
__attribute__((naked)) static void vv_hook_orig_tail() {
    asm volatile(
        "push %esi\n\t"             // the displaced `push esi`
        "lea 0xc(%esp),%ecx\n\t"    // the displaced `lea ecx,[esp+0xc]` — depends on ESP, so the
                                    // detour must leave the stack exactly as it found it
        "push $0x004aae58\n\t"      // resume at VV_HOOK_RESUME...
        "ret\n\t");                 // ...via `push addr; ret`, which needs no scratch register
}

// VV_HOOK_DETOUR(name, cb) defines the naked detour to pass to vv_install_hook. It calls
// cb(element) and then continues down the chain.
//
// pusha/pushf around the call is what makes chaining safe: the next detour (or the original
// code) sees every register, every flag, and ESP exactly as the game left them. popa restores
// EAX too, so the element is still in EAX for whoever comes next.
//
// cb must be `extern "C" void cb(uint32_t* el)` — the leading underscore below is mingw's 32-bit
// C name mangling.
#define VV_HOOK_DETOUR(name, cb)                \
    __attribute__((naked)) static void name() { \
        asm volatile(                           \
            "pusha\n\t"                         \
            "pushf\n\t"                         \
            "push %eax\n\t"                     \
            "call _" #cb "\n\t"                 \
            "add $4,%esp\n\t"                   \
            "popf\n\t"                          \
            "popa\n\t"                          \
            "jmp *_vv_hook_chain\n\t");         \
    }

// vv_install_hook writes the 5-byte jmp to `detour`, chaining onto any hook already there.
// Returns false if it refused or could not, in which case the caller should carry on without a
// hook rather than assume it worked.
static bool vv_install_hook(void* detour) {
    uint8_t* site = (uint8_t*)VV_HOOK_SITE;

    uint8_t saved[5];
    memcpy(saved, site, sizeof saved);

    if (saved[0] == 0xE9) {
        int32_t rel = 0;
        memcpy(&rel, saved + 1, sizeof rel);
        vv_hook_chain = (void*)(site + 5 + rel);     // resume into the mod that hooked first
    } else {
        static const uint8_t orig[5] = VV_HOOK_ORIG_BYTES;
        // Not the bytes we expect and not a hook we recognise. This is a THUG2.exe we were not
        // built against, and patching it would bind to whatever happens to live at this address.
        // Refuse: a mod that does nothing is repairable, a mod that corrupts a random function is
        // a bug report nobody can reproduce.
        if (memcmp(saved, orig, sizeof orig) != 0) return false;
        vv_hook_chain = (void*)&vv_hook_orig_tail;
    }

    uint8_t patch[5];
    patch[0] = 0xE9;                                                    // jmp rel32
    int32_t rel = (int32_t)((uint8_t*)detour - (site + 5));
    memcpy(patch + 1, &rel, sizeof rel);

    // PAGE_READWRITE, never PAGE_EXECUTE_READWRITE: macOS enforces W^X, so a page may not be
    // writable and executable at once and the RWX request simply FAILS there. The old code
    // ignored the return value and wrote into a still-read-only page.
    DWORD old = 0;
    if (!VirtualProtect(site, sizeof patch, PAGE_READWRITE, &old)) return false;
    memcpy(site, patch, sizeof patch);                                  // one store, fully formed
    DWORD tmp = 0;
    VirtualProtect(site, sizeof patch, old, &tmp);

    // Rosetta caches its translation of x86 it has already executed. Without this it can keep
    // running the OLD bytes even though memory now holds ours.
    FlushInstructionCache(GetCurrentProcess(), site, sizeof patch);
    return true;
}
