#!/usr/bin/env bash
# Build and run the vv_hook.h chain regression test (32-bit mingw, executed under wine).
#
# Guards the silent failure mode described in host.cpp: two VV mods hooking the same address, the
# second one clobbering the first's 5-byte jmp. Run this after ANY change to ../vv_hook.h.
set -euo pipefail
cd "$(dirname "$0")"

CXX=i686-w64-mingw32-g++
command -v "$CXX" >/dev/null || { echo "need $CXX (dnf install mingw32-gcc-c++)"; exit 1; }
command -v wine   >/dev/null || { echo "need wine to run the test"; exit 1; }

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

$CXX -O2 -shared -static -masm=att -o "$out/modA.dll" modA.cpp -lkernel32
$CXX -O2 -shared -static -masm=att -o "$out/modB.dll" modB.cpp -lkernel32

# The host must NOT live at the default 0x400000 image base: that is where THUG2.exe loads, so its
# own .text would sit on top of the hook site and VirtualAlloc could not reserve it. Rebase it out
# of the way and 0x4aae53 is free for the fake game code.
$CXX -O2 -static -masm=att -Wl,--image-base,0x10000000 -o "$out/host.exe" host.cpp -lkernel32

rc=0
for order in ab ba; do   # order must not matter — that is the point
  WINEDEBUG=-all wine "$out/host.exe" "$order" || rc=1
done
exit $rc
