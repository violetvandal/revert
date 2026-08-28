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
//  * PORTRAIT ONLY: the whole canvas is re-fitted to the frame width and then SLID DOWN
//    via the y offset so its bottom edge lands at VV_HUD_BOTTOM (default 0.88) of the
//    frame. That puts the balance meter, special bar and trick text down by the skater
//    where you read them, in one move, without knowing which element is which. The score
//    and goal points are then lifted back into the top corner, since they belong there.
//    Moving the_balance_meter (0xa4db8a4b) directly does NOT work -- it is resolved by the
//    hook but sits at (0,0), and panelstuff.qb re-sets its props constantly -- which is
//    why this slides the canvas instead of chasing individual elements.
//
// Runs alongside the stock WidescreenFix. No-ops if FixHUD is off (offset==0).
//
// The hook itself lives in ../common/vv_hook.h, because VV.KeyboardGrid hooks the SAME address
// and a second raw 5-byte jmp here would silently overwrite whichever of us loaded first. See
// that header — the chaining is the whole point of it.
#include <windows.h>
#include <cstdint>
#include <cstring>
#include <cstdlib>
#include "../common/vv_hook.h"

static int32_t* const HUD_OFFSET = (int32_t*)0x00786d88;
static float*   const HUD_SCALE  = (float*)  0x00786d80;
// The interface is a 640x480 canvas mapped to the screen with ONE SCALE PER AXIS, set at
// 0x4d88ce / 0x4d88ee:
//     [0x786d80] = width  * 1/640      (x)
//     [0x786d84] = height * 1/480      (y)
//     [0x786d8c] = y offset            (the game inits it to (int)(yscale * 16))
// so the two scales are only equal at 4:3, and touching one alone squashes every glyph
// by the ratio between them. The full transform, read off 0x4dff8c and 0x4e00ea:
//
//     x' = x * [0x786d80] + [0x786d88]
//     y' = y * [0x786d84] + [0x786d8c]
//
// 0x786d8c is therefore a global Y OFFSET, the exact counterpart of the pillarbox X one,
// which is what makes a vertical layout tractable: the canvas can be slid down as a whole
// instead of repositioning elements one at a time. That matters because element positions
// here are ABSOLUTE canvas coordinates, not parent-relative -- a container at (0,0) does
// not drag its children, which is why moving the_balance_meter moved nothing on screen.
static float*   const HUD_SCALE_Y = (float*)  0x00786d84;
static int32_t* const HUD_YOFF    = (int32_t*)0x00786d8c;
static const uint32_t ID_SCORE = 0x675adbaa;
static const uint32_t ID_GPTS  = 0xfa426fbe;
// the_balance_meter: the manual/grind/lip balance bar. Name recovered by checksumming
// candidates with the Neversoft StringToChecksum (crc32 of the lowercased name, no final
// XOR) and confirming the hit appears in qb_scripts.prx as
// "CreateScreenElement id the_balance_meter type parent". Its needle (balance_indicator)
// is parented to it and rides along, exactly like the score's star children.
static const uint32_t ID_BALANCE = 0xa4db8a4b;
// the_trick_text: the combo string. The game already WRAPS it at the canvas width, so its
// widest possible line is the full 640 units; shrinking the element by 1/fill makes that
// maximum exactly equal the visible width, and it can never run off the sides again. On
// screen it ends up the same size it would be at fill 1.0, because the canvas itself is
// scaled up by fill.
static const uint32_t ID_TRICK = 0x44727dae;
// The element's own scale lives at +0xe8/+0xec, and the engine derives a transform from
// it (0x4a94a0 recomputes +0xd8 and friends on every set). Writing the derived matrix does
// nothing -- it gets rebuilt -- so scaling means calling the setter the scripts call:
//
//   SetScale(this=ecx, float sx, float sy, bool relative, int)   __thiscall, ret 0x10
//
// found from the props parser reading the "scale" param at 0x4a9b82 and calling it.
static const uint32_t SET_SCALE = 0x004a94a0;

// Called from the HOOK, i.e. on the game's own thread while it is already inside the
// element code -- not from the worker, which would be re-entering the engine at random.
static void el_set_scale(void* el, float s) {
    if (!el) return;
    volatile float sx = s, sy = s;
    __asm__ __volatile__(
        "pushl $0\n\t"          // arg4
        "pushl $0\n\t"          // relative = 0, so this is an absolute scale
        "pushl %2\n\t"          // sy
        "pushl %1\n\t"          // sx  (pushed last => [esp+4] at entry)
        "movl %0, %%ecx\n\t"    // this
        "movl $0x004a94a0, %%eax\n\t"
        "call *%%eax\n\t"       // callee cleans its 16 bytes (ret 0x10)
        :
        : "r"(el), "m"(sx), "m"(sy)
        : "eax", "ecx", "edx", "memory", "cc");
}
static const int XO[4] = {0x90, 0x9c, 0xd4, 0xe0};
// The y twin of each x above. Verified in the exe: +0x90 and +0x94 are loaded together in
// the element's coordinate transform (e.g. at 0x425fb7).
static const int YO[4] = {0x94, 0xa0, 0xd8, 0xe4};

extern "C" volatile uint32_t* g_scr = nullptr;   // the_score element
extern "C" volatile uint32_t* g_gpd = nullptr;   // goal_points_display element (animated)
extern "C" volatile uint32_t* g_bal = nullptr;   // the_balance_meter (diagnostic only)
static float g_y_offset = 0.0f;                  // screen px pinned into HUD_YOFF
static float g_x_offset = 0.0f;                  // screen px pinned into HUD_OFFSET
static float g_fill     = 1.0f;                  // canvas scale multiplier past width-fit
extern "C" volatile uint32_t* g_trk = nullptr;   // the_trick_text
static float g_trick_scale = 0.0f;               // 1/fill; 0 = not engaged
static bool  g_trick_logged = false;
// VV_HUD_TRICK: off (default) | scale | parent
//   off    - leave the combo string alone. Long combos clip at high fill, but nothing
//            fights the game. This is the default because forcing the scale on the text
//            itself made it flicker: the element is animated (it pops on each trick), so
//            re-applying an absolute scale every time the game re-sets it visibly fought
//            that animation.
//   scale  - force 1/fill on the text element (fits perfectly, but flickers as above).
//   parent - scale the text's CONTAINER once instead, so the child's pop animation is
//            left alone. Untested; the container may also move the text slightly.
static int g_trick_mode = 2;                     // 0 off, 1 scale, 2 parent (default)
static const uint32_t ID_TRICK_PARENT = 0x1e55886e;
// pause_menu: the container the pause entries hang off. Identified by dumping every
// element the hook resolves, mapping those ids back to the script that declares them
// (gamemenu_pause.qb owns the nine rows at x=-125), and taking the container logged
// immediately before them -- which checksums exactly to "pause_menu".
//
// A full-width menu is the worst case for a canvas wider than the frame, so it gets the
// same 1/fill treatment as the combo text. Unlike the combo text this one is not animated,
// so setting it once does not fight anything. VV_HUD_MENU=0 turns it off.
static const uint32_t ID_PAUSE_MENU = 0x262ff1c8;
static bool g_menu_fit = true;
// VV_HUD_SCORE_UP: raise the score / SPECIAL / goal-points cluster by this fraction of the
// frame height. The canvas is a fixed 480-unit band and the score already sits at its top
// (canvas y=15), so a lift pushes it towards NEGATIVE canvas y. A big lift is known to make
// it disappear -- the 2D layer looks to be clipped to the canvas rect -- but a small one was
// never tested, so this exists to find the boundary: raise it until it vanishes, then back
// off. Whatever the largest working value turns out to be is the real answer.
// 0.18 of the frame height. Found by walking it up until the cluster disappeared: 0.18
// draws, 0.20 is gone, so the clip edge sits between them and this is as high as the score
// can go. VV_HUD_SCORE_UP overrides; 0 disables.
// VV_HUD_HIDE: comma-separated hex element ids to push off the canvas, e.g.
//   VV_HUD_HIDE=0xc9cc7de7
// Cutscenes draw letterbox bars at the CANVAS top and bottom. Normally that is the screen
// top and bottom, but a portrait canvas is an inset band, so the bars land across the
// middle of the picture. They are ordinary screen elements, so shoving them far above the
// canvas makes the clip swallow them. Portrait only, and it never destroys anything -- the
// element is simply parked outside the drawable rect.
static const int MAX_HIDE = 8;
static uint32_t g_hide_id[MAX_HIDE];
static int g_hide_n = 0;
static volatile uint32_t* g_hide_el[MAX_HIDE];
static float g_hide_y[MAX_HIDE];
static int g_hide_mask[MAX_HIDE];

static const float SCORE_UP_DEFAULT = 0.18f;
static float g_score_up = 0.0f;                  // canvas units, 0 = leave alone
static float g_score_y  = 0.0f;                  // score's corrected y; 0 = not captured
static float g_gpts_y   = 0.0f;                  // same for the goal-points container
// Which of the four YO slots actually held this element's y when we first saw it. The x
// code has always self-selected the same way ("if (*x == 136.0f)"), and that guard is
// load-bearing: the four offsets are position copies for SOME element types, but for
// others a slot holds a dimension or an alpha, and writing a y into it hides the element
// instead of moving it. Capture the mask once, then only ever write those slots.
static int g_score_mask = 0;
static int g_gpts_mask  = 0;

static bool readable(const void* p, size_t n);   // defined below

static int y_mask(const uint32_t* el, float raw) {
    int m = 0;
    for (int i = 0; i < 4; i++)
        if (*(const float*)((const uint8_t*)el + YO[i]) == raw) m |= 1 << i;
    return m;
}

static void pin_y(volatile uint32_t* p, uint32_t id, int mask, float target) {
    uint32_t* el = (uint32_t*)p;
    if (!el || !mask || !readable(el, 0x100) || el[0x14/4] != id) return;
    for (int i = 0; i < 4; i++)
        if (mask & (1 << i)) {
            float* y = (float*)((uint8_t*)el + YO[i]);
            if (*y != target) *y = target;
        }
}

// readable(p,n) — is p..p+n committed and readable?
//
// NOT IsBadReadPtr. IsBadReadPtr validates by faulting and catching, and on Apple Silicon every
// fault is a Mach round-trip through Rosetta's exception path. The worker below runs at 250 Hz,
// so the old code drove that fault path a thousand times a second — and the crash log shows this
// thread taking an access violation inside the probe and unwinding into "Exception frame is not
// in stack limits". VirtualQuery asks the kernel the same question without ever faulting.
static bool readable(const void* p, size_t n) {
    MEMORY_BASIC_INFORMATION mbi;
    if (!VirtualQuery(p, &mbi, sizeof mbi)) return false;
    if (mbi.State != MEM_COMMIT) return false;
    if (mbi.Protect & (PAGE_NOACCESS | PAGE_GUARD)) return false;
    const uint8_t* base = (const uint8_t*)mbi.BaseAddress;
    return (const uint8_t*)p + n <= base + mbi.RegionSize;   // no straddle into the next region
}

static void logline(const char* fmt, int a, int b, int c, int d, int e, int f);  // defined below

// ---- portrait fit ------------------------------------------------------------------
// FixHUD sizes the HUD canvas off the screen HEIGHT: a 4:3 box, pillarboxed, so
// HUD_OFFSET = (W - H*4/3)/2 and HUD_SCALE maps canvas units to screen pixels. That is
// fine while the frame is 4:3 or WIDER. On a frame NARROWER than 4:3 the same rule makes
// the canvas wider than the screen -- at 1080x1920 it is 2560 wide on a 1080 screen,
// 740px off each side -- which is exactly why the pause menu and long trick strings run
// off the sides of a vertical render.
//
// Re-fit the canvas to the WIDTH instead: put BOTH scales on width/640 and drop the
// offset to 0, so canvas x=0 is the true left edge and x=640 the right one. Both axes get
// the same number, which is the whole point -- an x-only change is what compresses the
// glyphs horizontally into tall thin letters. Nothing is clipped and nothing is distorted.
//
// The trade is vertical: a 4:3 canvas cannot cover a 9:16 frame, so at width-fit it
// occupies the top width*3/4 rows and bottom-anchored elements sit there rather than at
// the very bottom of the frame.
//
// Gated on aspect < 4:3, so every normal (widescreen / ultrawide) launch is untouched.
// Set VV_HUD_PORTRAIT=0 to disable and see the stock behaviour.
static float g_portrait_scale = 0.0f;   // value we pin into HUD_SCALE; 0 = not engaged
static bool  g_portrait_done  = false;  // engaged (or decided not to) -- never recompute

static bool portrait_disabled() {
    char buf[8];
    DWORD n = GetEnvironmentVariableA("VV_HUD_PORTRAIT", buf, sizeof buf);
    return n == 1 && buf[0] == '0';
}

static void portrait_init() {
    if (g_portrait_done) return;
    if (!readable(HUD_SCALE, 4) || !readable(HUD_OFFSET, 4)) return;
    const float s = *HUD_SCALE;
    if (s <= 0.01f) return;              // fix has not computed it yet; try again next tick
    g_portrait_done = true;              // from here on, decided
    if (portrait_disabled()) return;

    const int W = GetSystemMetrics(SM_CXSCREEN), H = GetSystemMetrics(SM_CYSCREEN);
    if (W <= 0 || H <= 0) return;
    if (W * 3 >= H * 4) return;          // 4:3 or wider: the stock behaviour is correct

    // Straight from the game's own formula, so this holds whether FixHUD is on or off.
    // VV_HUD_FILL scales PAST width-fit. The 2D layer is clipped to the 640x480 canvas
    // rect, so the canvas is the only place a HUD element can exist: at fill 1.0 that band
    // is just 42% of a 9:16 frame, which forces a choice between the score at the top and
    // the balance meter down by the skater. Scaling up makes the band taller so both fit,
    // at the cost of the canvas being wider than the frame -- gameplay HUD sits well
    // inside, full-width menus are what start to lose their edges.
    float fill = 1.0f;
    char fbuf[16];
    DWORD fn = GetEnvironmentVariableA("VV_HUD_FILL", fbuf, sizeof fbuf);
    if (fn > 0 && fn < sizeof fbuf) {
        fbuf[fn] = '\0';
        const float v = (float)atof(fbuf);
        if (v >= 1.0f && v <= 2.5f) fill = v;
    }
    g_fill = fill;
    // The game wraps the combo string at the canvas width, so its widest line is 640 units;
    // at 1/fill that maximum equals the visible width exactly and it can never clip.
    if (fill > 1.0f) {
        char tbuf[16];
        DWORD tn = GetEnvironmentVariableA("VV_HUD_TRICK", tbuf, sizeof tbuf);
        if (tn > 0 && tn < sizeof tbuf) {
            tbuf[tn] = '\0';
            if (tbuf[0] == 's') g_trick_mode = 1;
            else if (tbuf[0] == 'p') g_trick_mode = 2;
            else if (tbuf[0] == 'o' || tbuf[0] == '0') g_trick_mode = 0;
        }
        if (g_trick_mode) g_trick_scale = 1.0f / fill;
        char mbuf[8];
        DWORD mn = GetEnvironmentVariableA("VV_HUD_MENU", mbuf, sizeof mbuf);
        if (mn == 1 && mbuf[0] == '0') g_menu_fit = false;
    }
    g_portrait_scale = ((float)W / 640.0f) * fill;
    // Centre the canvas horizontally, so anything the scripts centre stays centred and the
    // overflow is split evenly instead of all falling off the right.
    g_x_offset = ((float)W - 640.0f * g_portrait_scale) * 0.5f;

    // Width-fit leaves the canvas covering only the top width*3/4 rows, stranding
    // everything bottom-anchored (balance meter, special bar, trick text) up by the score
    // instead of down by the skater. Slide the whole canvas down so its bottom edge lands
    // at VV_HUD_BOTTOM of the frame, using the game's own y offset. One global move, so it
    // does not care which element is which -- the lesson from trying to move the balance
    // meter directly and finding it is a (0,0) container that drags nothing.
    float bottom = 0.88f;                        // canvas bottom, as a fraction of the frame
    char buf[16];
    DWORD n = GetEnvironmentVariableA("VV_HUD_BOTTOM", buf, sizeof buf);
    if (n > 0 && n < sizeof buf) {
        buf[n] = '\0';
        const float v = (float)atof(buf);
        if (v > 0.30f && v <= 1.0f) bottom = v;  // tune between takes, no rebuild
    }
    char hbuf[80];
    DWORD hn = GetEnvironmentVariableA("VV_HUD_HIDE", hbuf, sizeof hbuf);
    if (hn > 0 && hn < sizeof hbuf) {
        hbuf[hn] = '\0';
        for (const char* q = hbuf; *q && g_hide_n < MAX_HIDE; ) {
            while (*q == ',' || *q == ' ') q++;
            if (!*q) break;
            g_hide_id[g_hide_n++] = (uint32_t)strtoul(q, nullptr, 0);
            while (*q && *q != ',') q++;
        }
        logline("hide: %d id(s), first 0x%08x\r\n", g_hide_n, g_hide_n ? g_hide_id[0] : 0, 0,0,0,0);
    }

    float up = SCORE_UP_DEFAULT;
    char ubuf[16];
    DWORD un = GetEnvironmentVariableA("VV_HUD_SCORE_UP", ubuf, sizeof ubuf);
    if (un > 0 && un < sizeof ubuf) {
        ubuf[un] = '\0';
        const float v = (float)atof(ubuf);
        if (v >= 0.0f && v < 0.5f) up = v;
    }
    if (up > 0.0f) g_score_up = up * (float)H / g_portrait_scale;
    g_y_offset = bottom * (float)H - 480.0f * g_portrait_scale;
    // No score lift: anything placed above the canvas rect is CLIPPED, not drawn, which is
    // what made the score and goal points vanish. Everything has to live inside the band,
    // so the band's placement is the whole layout.
    logline("portrait: W=%d H=%d scale/1000=%d xoff=%d yoff=%d band=%d\r\n",
            W, H, (int)(g_portrait_scale * 1000.0f), (int)g_x_offset, (int)g_y_offset,
            (int)(480.0f * g_portrait_scale));
}

static bool hud_voff(float* out) {
    if (!readable(HUD_OFFSET, 4) || !readable(HUD_SCALE, 4)) return false;
    float scale = *HUD_SCALE; int32_t off = *HUD_OFFSET;
    if (scale <= 0.01f || off == 0) return false;
    *out = (float)off / scale; return true;
}

// ---- element dump (diagnostic) -------------------------------------------------------
// VV_HUD_DUMP=1 appends every DISTINCT element id this hook resolves, with the x/y we
// would rewrite, to vv_hudfix.log in the game directory.
//
// This exists because a checksum proving a name is real does NOT prove the hook ever sees
// it: we only get elements that something looks up BY ID. An element the scripts create
// once and then drive through a stored pointer never comes past here, so the capture stays
// null and the reposition silently does nothing. Play the thing you want to move, then
// read the log to see what actually flows through.
static uint32_t g_seen[4096];
// Baselined at load: GetTickCount() is time since the machine booted, not since the game
// started, so comparing it against a raw 30000 is only ever true on a freshly booted box.
static DWORD g_t0 = 0;
static int g_nseen = 0;
static int g_dump = -1;   // -1 unknown, 0 off, 1 on

static void logline(const char* fmt, int a, int b, int c, int d, int e, int f) {
    char line[192];
    const int len = wsprintfA(line, fmt, a, b, c, d, e, f);
    HANDLE h = CreateFileA("vv_hudfix.log", FILE_APPEND_DATA, FILE_SHARE_READ, nullptr,
                           OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (h != INVALID_HANDLE_VALUE) {
        DWORD wrote = 0; SetFilePointer(h, 0, nullptr, FILE_END);
        WriteFile(h, line, len, &wrote, nullptr); CloseHandle(h);
    }
}

static void dump_element(uint32_t id, const uint32_t* el) {
    if (g_dump < 0) {
        char buf[8];
        DWORD n = GetEnvironmentVariableA("VV_HUD_DUMP", buf, sizeof buf);
        g_dump = (n == 1 && buf[0] == '1') ? 1 : (n == 1 && buf[0] == '2') ? 2 : 0;
    }
    if (!g_dump || g_nseen >= (int)(sizeof g_seen / sizeof *g_seen)) return;
    // Mode 2 ignores the first 30 seconds. Prefer mode 1 with the larger buffer: the menu
    // then simply lands at the END of the log, no timing required.
    if (g_dump == 2 && (GetTickCount() - g_t0) < 30000) return;
    for (int i = 0; i < g_nseen; i++) if (g_seen[i] == id) return;
    g_seen[g_nseen++] = id;

    float x = 0, y = 0, sc = 0;
    if (readable(el, 0x120)) {
        x  = *(const float*)((const uint8_t*)el + 0x90);
        y  = *(const float*)((const uint8_t*)el + 0x94);
        sc = *(const float*)((const uint8_t*)el + 0xe8);
    }
    char line[160];
    const int len = wsprintfA(line, "id=0x%08x  x=%d  y=%d  scale/1000=%d\r\n",
                              id, (int)x, (int)y, (int)(sc * 1000.0f));
    HANDLE h = CreateFileA("vv_hudfix.log", FILE_APPEND_DATA, FILE_SHARE_READ, nullptr,
                           OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (h != INVALID_HANDLE_VALUE) {
        DWORD wrote = 0;
        SetFilePointer(h, 0, nullptr, FILE_END);
        WriteFile(h, line, len, &wrote, nullptr);
        CloseHandle(h);
    }
}

extern "C" void hud_on_resolve(uint32_t* el) {
    if (!el) return;
    portrait_init();          // idempotent; the worker may not have run yet
    uint32_t id = el[0x14/4];
    for (int i = 0; i < g_hide_n; i++) {
        if (id != g_hide_id[i] || !readable(el, 0x120)) continue;
        g_hide_el[i] = el;
        if (g_hide_y[i] == 0.0f) {
            const float raw = *(float*)((uint8_t*)el + YO[0]);
            g_hide_mask[i] = y_mask(el, raw);
            g_hide_y[i] = raw - 4000.0f;      // far outside the canvas, so the clip eats it
            logline("hide 0x%08x: y %d -> %d slots=0x%x\r\n", id, (int)raw,
                    (int)g_hide_y[i], g_hide_mask[i], 0, 0);
        }
    }
    dump_element(id, el);
    if (id == ID_SCORE) {
        g_scr = el;
        if (g_score_up > 0.0f && g_score_y == 0.0f && readable(el, 0x120)) {
            const float raw = *(float*)((uint8_t*)el + YO[0]);
            g_score_mask = y_mask(el, raw);
            g_score_y = raw - g_score_up;
            logline("score up: %d -> %d (screen %d) slots=0x%x\r\n", (int)raw, (int)g_score_y,
                    (int)(g_score_y * g_portrait_scale + g_y_offset), g_score_mask, 0, 0);
        }
        // This correction is what keeps the score inside the frame in BOTH directions. On
        // ultrawide the offset is positive and it pulls the plate left, out of the
        // pillarbox. In portrait with fill > 1 the canvas is wider than the frame, so the
        // offset is NEGATIVE and the very same expression pushes the plate right by exactly
        // the overhang, landing its left edge on screen x = 0. Disabling it here was wrong.
        float voff; if (!hud_voff(&voff)) return;
        for (int i=0;i<4;i++){ float* x=(float*)((uint8_t*)el+XO[i]); if (*x==136.0f) *x=136.0f-voff; }
    } else if (id == ID_GPTS) {
        g_gpd = el;
        if (g_score_up > 0.0f && g_gpts_y == 0.0f && readable(el, 0x120)) {
            const float raw = *(float*)((uint8_t*)el + YO[0]);
            g_gpts_mask = y_mask(el, raw);
            g_gpts_y = raw - g_score_up;
        }                                   // remember it; worker thread pins it
    } else if (id == ID_PAUSE_MENU) {
        if (g_menu_fit && g_fill > 1.0f && g_portrait_scale > 0.0f && readable(el, 0x120)) {
            const float target = 1.0f / g_fill;
            float* cur = (float*)((uint8_t*)el + 0xe8);
            if (*cur != target) {
                el_set_scale(el, target);
                logline("menu: scale %d/1000 -> %d/1000\r\n",
                        (int)(*cur * 1000.0f), (int)(target * 1000.0f), 0, 0, 0, 0);
            }
        }
    } else if (id == ID_TRICK_PARENT && g_trick_mode == 2) {
        // Scale the container once. Its children, the combo string among them, come along,
        // and their own animations keep running relative to it.
        if (g_trick_scale > 0.0f && readable(el, 0x120)) {
            float* cur = (float*)((uint8_t*)el + 0xe8);
            if (*cur != g_trick_scale) el_set_scale(el, g_trick_scale);
        }
    } else if (id == ID_TRICK) {
        g_trk = el;
        if (g_trick_mode == 1 && g_trick_scale > 0.0f && readable(el, 0x120)) {
            float* cur = (float*)((uint8_t*)el + 0xe8);
            if (*cur != g_trick_scale) {
                if (!g_trick_logged) {
                    g_trick_logged = true;
                    logline("trick: scale %d/1000 -> %d/1000 (fill %d/100)\r\n",
                            (int)(*cur * 1000.0f), (int)(g_trick_scale * 1000.0f),
                            (int)(g_fill * 100.0f), 0, 0, 0);
                }
                el_set_scale(el, g_trick_scale);
            }
        }
    } else if (id == ID_BALANCE) {
        g_bal = el;                                   // ditto: it is created per manual/grind
    }
}

VV_HOOK_DETOUR(hud_detour, hud_on_resolve)           // hook body at 0x4aae53 (eax = element)

static DWORD WINAPI worker(LPVOID) {
    // Pin the animated goal-points container to the left. No code patching here any more.
    for (;;) {
        Sleep(4);
        // Pinned rather than written once: the fix recomputes these on a device reset,
        // and a one-shot write would be silently undone by the next one.
        portrait_init();
        if (g_portrait_scale > 0.0f && readable(HUD_SCALE, 4) && readable(HUD_SCALE_Y, 4)
            && readable(HUD_OFFSET, 4) && readable(HUD_YOFF, 4)) {
            const int32_t yoff = (int32_t)g_y_offset;
            if (*HUD_SCALE   != g_portrait_scale) *HUD_SCALE   = g_portrait_scale;
            if (*HUD_SCALE_Y != g_portrait_scale) *HUD_SCALE_Y = g_portrait_scale;
            if (*HUD_OFFSET  != (int32_t)g_x_offset) *HUD_OFFSET = (int32_t)g_x_offset;
            if (*HUD_YOFF    != yoff)             *HUD_YOFF    = yoff;
        }
        for (int i = 0; i < g_hide_n; i++)
            if (g_hide_y[i] != 0.0f) pin_y(g_hide_el[i], g_hide_id[i], g_hide_mask[i], g_hide_y[i]);

        if (g_score_up > 0.0f) {
            pin_y(g_scr, ID_SCORE, g_score_mask, g_score_y);
            pin_y(g_gpd, ID_GPTS,  g_gpts_mask,  g_gpts_y);
        }

        uint32_t* el = (uint32_t*)g_gpd;
        if (!el || !readable(el, 0x100)) continue;
        if (el[0x14/4] != ID_GPTS) { g_gpd = nullptr; continue; }   // freed/reused -> drop
        float voff; if (!hud_voff(&voff)) continue;
        float target = 0.0f - voff;                                  // raw X is 0 (canvas left)
        for (int i=0;i<4;i++){ float* x=(float*)((uint8_t*)el+XO[i]); if (*x != target) *x = target; }
    }
    return 0;
}

BOOL APIENTRY DllMain(HMODULE h, DWORD r, LPVOID) {
    if (r == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(h);
        g_t0 = GetTickCount();
        vv_install_hook((void*)&hud_detour);   // cold, before any game thread runs; chains if
                                               // VV.KeyboardGrid already hooked the same site
        CreateThread(0,0,worker,0,0,0);        // pinning only
    }
    return TRUE;
}
