package core

import "syscall"

// Explorer caches icons per file. A user who updates in place keeps seeing the old icon
// (or a blank one, for binaries built before we shipped a resource) on an existing
// shortcut, even after the .lnk is rewritten. SHChangeNotify tells the shell that
// associations changed, which drops the cached icons.
//
// Called via a direct Win32 API rather than by spawning `ie4uinit.exe -show`: this process
// launches no child, so Defender's behavior monitor has nothing to flag. See the LOLBin
// guard in .github/workflows/windows-defender-scan.yml.
var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procSHChangeNotify = shell32.NewProc("SHChangeNotify")
)

const (
	shcneAssocChanged = 0x08000000
	shcnfIDList       = 0x0000
)

func refreshShellIcons() {
	// Best effort: a failure here costs a stale icon, never a failed install.
	procSHChangeNotify.Call(uintptr(shcneAssocChanged), uintptr(shcnfIDList), 0, 0)
}
