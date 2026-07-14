// Test mod A — a stand-in for one VV .asi. See host.cpp.
#include <windows.h>
#include <cstdint>
#include "../vv_hook.h"

extern "C" __declspec(dllexport) volatile long     a_hits = 0;
extern "C" __declspec(dllexport) volatile uint32_t a_last = 0;

extern "C" void a_on_resolve(uint32_t* el) { a_hits++; a_last = (uint32_t)el; }
VV_HOOK_DETOUR(a_detour, a_on_resolve)

// Installed explicitly by the host rather than from DllMain, so the test controls the ORDER the
// two mods hook in (which is the whole thing under test) and can see the return value.
extern "C" __declspec(dllexport) int a_install() { return vv_install_hook((void*)&a_detour) ? 1 : 0; }

BOOL APIENTRY DllMain(HMODULE, DWORD, LPVOID) { return TRUE; }
