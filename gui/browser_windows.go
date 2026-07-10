//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// openBrowser opens url in the default browser via ShellExecuteW ("open" verb) — the same
// call the shell makes when you click a link. Crucially it spawns NO child process
// (no rundll32 / cmd start), so Windows Defender's behavior monitor doesn't flag the
// (unsigned) GUI as Behavior:Win32/DefenseEvasion for proxy-executing a LOLBin.
func openBrowser(url string) {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecuteW := shell32.NewProc("ShellExecuteW")
	verb, _ := syscall.UTF16PtrFromString("open")
	target, _ := syscall.UTF16PtrFromString(url)
	const swShowNormal = 1
	shellExecuteW.Call(
		0, // hwnd
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(target)),
		0, // params
		0, // dir
		swShowNormal,
	)
}
