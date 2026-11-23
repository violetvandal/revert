// VV.CrashLog.asi — on the first access violation inside THUG2.exe's code, write the
// registers, a raw stack dump and memory at each register to vv_crash.log (game dir),
// then let the crash proceed normally. Diagnostic only; drop it in scripts/ to enable.
// Build: i686-w64-mingw32-gcc -O2 -shared -o VV.CrashLog.asi crashlog.c -static-libgcc
#include <windows.h>
#include <stdio.h>

static volatile LONG fired = 0;

static int readable(DWORD p, DWORD n) {
    MEMORY_BASIC_INFORMATION mbi;
    if (p < 0x10000) return 0;
    if (!VirtualQuery((void *)p, &mbi, sizeof mbi)) return 0;
    if (mbi.State != MEM_COMMIT) return 0;
    if (mbi.Protect & (PAGE_NOACCESS | PAGE_GUARD)) return 0;
    return (DWORD)mbi.BaseAddress + mbi.RegionSize >= p + n;
}

static void dump(FILE *f, const char *tag, DWORD p, int words) {
    fprintf(f, "\n[%s @ %08lx]\n", tag, p);
    if (!readable(p, words * 4)) { fprintf(f, "  (unreadable)\n"); return; }
    for (int i = 0; i < words; i++) {
        if (i % 8 == 0) fprintf(f, "  +%03x:", i * 4);
        fprintf(f, " %08lx", ((DWORD *)p)[i]);
        if (i % 8 == 7) fprintf(f, "\n");
    }
}

static LONG CALLBACK handler(EXCEPTION_POINTERS *ep) {
    EXCEPTION_RECORD *er = ep->ExceptionRecord;
    CONTEXT *c = ep->ContextRecord;
    if (er->ExceptionCode != EXCEPTION_ACCESS_VIOLATION) return EXCEPTION_CONTINUE_SEARCH;
    if (c->Eip < 0x401000 || c->Eip > 0x700000) return EXCEPTION_CONTINUE_SEARCH;
    if (InterlockedExchange(&fired, 1)) return EXCEPTION_CONTINUE_SEARCH;

    FILE *f = fopen("vv_crash.log", "w");
    if (!f) return EXCEPTION_CONTINUE_SEARCH;
    fprintf(f, "VV.CrashLog: access violation (%s %08lx) at eip=%08lx\n",
            er->ExceptionInformation[0] ? "write" : "read",
            (DWORD)er->ExceptionInformation[1], c->Eip);
    fprintf(f, "eax=%08lx ebx=%08lx ecx=%08lx edx=%08lx\nesi=%08lx edi=%08lx ebp=%08lx esp=%08lx\n",
            c->Eax, c->Ebx, c->Ecx, c->Edx, c->Esi, c->Edi, c->Ebp, c->Esp);

    // Raw stack, plus a filtered list of words that look like return addresses into .text.
    DWORD *sp = (DWORD *)c->Esp;
    fprintf(f, "\n[code-looking stack words]\n");
    for (int i = 0; i < 2048 && readable((DWORD)(sp + i), 4); i++)
        if (sp[i] >= 0x401000 && sp[i] < 0x640000)
            fprintf(f, "  esp+%04x: %08lx\n", i * 4, sp[i]);
    dump(f, "stack", c->Esp, 512);
    dump(f, "eax", c->Eax, 32); dump(f, "ebx", c->Ebx, 32); dump(f, "ecx", c->Ecx, 32);
    dump(f, "edx", c->Edx, 32); dump(f, "esi", c->Esi, 32); dump(f, "ebp", c->Ebp, 32);
    // the function's first argument (the object whose +0x10 chain was null)
    if (readable(c->Esp + 0x50, 4)) dump(f, "arg0 (esp+0x50)", *(DWORD *)(c->Esp + 0x50), 32);
    fclose(f);
    return EXCEPTION_CONTINUE_SEARCH;
}

BOOL WINAPI DllMain(HINSTANCE h, DWORD reason, LPVOID r) {
    if (reason == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(h);
        AddVectoredExceptionHandler(1, handler);
    }
    return TRUE;
}
