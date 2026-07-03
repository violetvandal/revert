// VV.GlyphTest — experiment: make THUG2 PC render face-button glyph tokens (\b0..\b3 =
// Triangle/Square/Circle/Cross) as CONTROLLER GLYPHS instead of the bound keyboard key ("kp2").
//
// The font text renderer routes glyph indices 0-3 (face buttons) to the keyboard-key-name path,
// while 4-7 (d-pad) draw as glyphs. The gate is a `cmp al,4 / jb <keyname>` at two render sites:
//   0x4ced6f : 72 0c (jb 0x4ced7d)
//   0x4cff38 : 72 0c (jb 0x4cff46)
// NOP'ing those `jb`s lets indices 0-3 fall through to the glyph-draw path (the next checks
// `cmp al,7 / jbe <glyph>` then catch them). If the ButtonsXbox font has the face-button glyphs
// at slots 0-3, trick combos ("\b4 + \b2") should now show arrow + Circle GLYPH instead of "kp2".
//
// Pure 2-byte NOP patches at 2 sites; reversible by removing this .asi. Always-on for the test.
#include <windows.h>
#include <cstdint>

static void nop2(uint32_t va) {
    uint8_t* p = (uint8_t*)(uintptr_t)va;
    DWORD old;
    VirtualProtect(p, 2, PAGE_EXECUTE_READWRITE, &old);
    if (p[0] == 0x72 && p[1] == 0x0c) { p[0] = 0x90; p[1] = 0x90; }   // jb rel8 -> nop nop
    VirtualProtect(p, 2, old, &old);
}

static DWORD WINAPI worker(LPVOID) {
    Sleep(8000);
    nop2(0x004ced6f);
    nop2(0x004cff38);
    return 0;
}

BOOL APIENTRY DllMain(HMODULE h, DWORD r, LPVOID) {
    if (r == DLL_PROCESS_ATTACH) { DisableThreadLibraryCalls(h); CreateThread(0, 0, worker, 0, 0, 0); }
    return TRUE;
}
