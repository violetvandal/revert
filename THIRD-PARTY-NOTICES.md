# Third-party notices

Revert is MIT licensed (see `LICENSE`). That covers the code in this repository that we wrote.
It does **not** cover the third-party components listed below, which we redistribute under their
own licenses, and which remain the copyright of their respective authors.

Revert also downloads some things at install time rather than redistributing them (Wine, MoltenVK,
the Go toolchain, THUG Pro). Those are fetched from their own publishers, are never bundled here,
and so are not listed as redistributed components.

Revert never ships THUG2 game files. You supply your own copy of the game.

---

## DXVK (modified)

- **Shipped as:** `tools/dxvk-mac/d3d9-dxvk-patched-m1.dll`
- **Upstream:** https://github.com/doitsujin/dxvk, via the macOS fork https://github.com/Gcenx/DXVK-macOS
- **License:** zlib/libpng

> Copyright (c) 2017 Philip Rebohle
> Copyright (c) 2019 Joshua Ashton
> Copyright (c) 2019 Robin Kertels
> Copyright (c) 2023 Jeffrey Ellison

### ⚠️ This is an ALTERED version of DXVK, not the original software

The zlib license asks that altered versions be plainly marked as such, so, plainly: **the DLL we
ship is not stock DXVK and the DXVK authors are not responsible for it.** It is a 32-bit build of
DXVK 1.10.3 (from the `1.10.x` branch of Gcenx/DXVK-macOS) with six patches of ours applied, which
let Direct3D 9 run on Apple Silicon and Intel Macs through MoltenVK, whose Metal backend exposes no
geometry shaders. Bugs you hit in this build are ours to answer for, so please report them to us and
not to DXVK upstream.

Our patches are in `tools/dxvk-mac/dxvk-apple-silicon-d3d9.patch`, and `tools/dxvk-mac/README.md`
has the exact commands to rebuild the DLL from upstream source. Nothing in our build is hidden from
you.

### zlib/libpng license

This software is provided 'as-is', without any express or implied warranty. In no event will the
authors be held liable for any damages arising from the use of this software.

Permission is granted to anyone to use this software for any purpose, including commercial
applications, and to alter it and redistribute it freely, subject to the following restrictions:

1. The origin of this software must not be misrepresented; you must not claim that you wrote the
   original software. If you use this software in a product, an acknowledgment in the product
   documentation would be appreciated but is not required.
2. Altered source versions must be plainly marked as such, and must not be misrepresented as being
   the original software.
3. This notice may not be removed or altered from any source distribution.

---

## WidescreenFixesPack (Tony Hawk's Underground 2 widescreen fix)

- **Shipped as:** `Game/scripts/TonyHawksUnderground2.WidescreenFix.asi` inside
  `tools/TonyHawksUnderground2.WidescreenFix.zip`, redistributed unmodified
- **Upstream:** https://github.com/ThirteenAG/WidescreenFixesPack
- **License:** MIT

> MIT License
> Copyright (c) 2018 ThirteenAG

This is what gives the edition its widescreen support. We configure it, we do not modify it.

---

## Ultimate ASI Loader

- **Shipped as:** `Game/dinput8.dll` inside `tools/TonyHawksUnderground2.WidescreenFix.zip`, and as
  the `winmm.dll` proxy in the built edition. Redistributed unmodified.
- **Upstream:** https://github.com/ThirteenAG/Ultimate-ASI-Loader
- **License:** MIT

> MIT License
> Copyright (c) 2023 ThirteenAG

This is the loader that lets THUG2 load `.asi` plugins at all, including our own
(`VV.HudFix`, `VV.GlyphFix`, `VV.KeyboardGrid`) and the widescreen fix above. The whole mod lane
depends on it.

---

## Full license texts

The complete license text for each component ships with its upstream project at the URLs above.
The zlib license, being short and carrying an explicit "may not be removed" clause, is reproduced
in full here.
