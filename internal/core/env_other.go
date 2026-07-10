//go:build !windows

package core

// Stubs so the package compiles on Linux/macOS. On those platforms every command
// delegates to the bash dispatcher (see delegate.go); ComputeStatus/Doctor and these
// helpers are only exercised natively on Windows.

func directXPresent() bool   { return false }
func thugProInstalled() bool { return false }
func thugProDir() string     { return "" }
func pad0Configured() bool   { return false }
