//go:build !windows

package core

// refreshShellIcons is a Windows-only concern (see shellnotify_windows.go). On Linux the
// bash dispatcher already runs update-desktop-database after writing the .desktop file.
func refreshShellIcons() {}
