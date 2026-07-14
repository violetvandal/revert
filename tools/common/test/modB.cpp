// Test mod B — a stand-in for the other VV .asi. Identical to modA but with its own counters, so
// the host can prove BOTH detours ran. See host.cpp.
#include <windows.h>
#include <cstdint>
#include "../vv_hook.h"

extern "C" __declspec(dllexport) volatile long     b_hits = 0;
extern "C" __declspec(dllexport) volatile uint32_t b_last = 0;

extern "C" void b_on_resolve(uint32_t* el) { b_hits++; b_last = (uint32_t)el; }
VV_HOOK_DETOUR(b_detour, b_on_resolve)

extern "C" __declspec(dllexport) int b_install() { return vv_install_hook((void*)&b_detour) ? 1 : 0; }

BOOL APIENTRY DllMain(HMODULE, DWORD, LPVOID) { return TRUE; }
