//go:build windows

// vv-padbridge — the native-Windows replacement for the Linux evdev trigger bridge
// (tools/trigger-bridge/thug2-trigger-bridge.py). It recreates the PS2 shoulder/trigger
// behaviour THUG2 can't bind natively: it polls the Xbox pad via XInput and synthesizes
// the game's keyboard keys via SendInput (DirectInput scan codes), so the same k0_ binds
// the game already uses fire.
//
//	LT (L2) -> KP7   Nollie / rotate-left      (hold)
//	RT (R2) -> KP9   Switch / rotate-right     (hold)
//	LB (L1) -> KP7   spin left                 (hold)
//	RB (R1) -> KP9   spin right                (hold)
//	LB+RB   -> KP1   get off board / walk      (suppresses the spin keys)
//
// Sticks and face buttons stay native (DirectInput reads the pad directly). Pure Go via
// syscalls — no CGO, no driver, no admin. Started/stopped by `revert run` around the game.
package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// XInput button masks.
const (
	xinputLeftShoulder  = 0x0100
	xinputRightShoulder = 0x0200
	errDeviceNotConn    = 1167 // ERROR_DEVICE_NOT_CONNECTED
)

// Trigger hysteresis (0..255), matching the evdev bridge.
const (
	trigOn  = 100
	trigOff = 60
)

// DirectInput (set-1) scan codes for the numpad keys the game binds.
const (
	scanKP7 = 0x47
	scanKP9 = 0x49
	scanKP1 = 0x4F
)

// SendInput flags.
const (
	inputKeyboard     = 1
	keyeventfKeyUp    = 0x0002
	keyeventfScancode = 0x0008
)

type xinputGamepad struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

type xinputState struct {
	PacketNumber uint32
	Gamepad      xinputGamepad
}

// keybdInput is KEYBDINPUT; input is INPUT laid out for amd64 (union padded to 32 bytes
// so sizeof(INPUT)==40).
type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type input struct {
	inputType uint32
	_         uint32 // align union to 8 bytes
	ki        keybdInput
	_         [8]byte // pad union (KEYBDINPUT=24) up to MOUSEINPUT=32
}

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
	xiGetState    = loadXInput()
)

// loadXInput resolves XInputGetState from whichever XInput DLL is available (1_4 on
// Win8+, then 1_3, then the always-present 9_1_0).
func loadXInput() *syscall.LazyProc {
	for _, dll := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
		d := syscall.NewLazyDLL(dll)
		if d.Load() == nil {
			return d.NewProc("XInputGetState")
		}
	}
	return nil
}

// getState reads controller idx; ok is false if it isn't connected.
func getState(idx uint32) (xinputState, bool) {
	var st xinputState
	r, _, _ := xiGetState.Call(uintptr(idx), uintptr(unsafe.Pointer(&st)))
	if r == errDeviceNotConn {
		return st, false
	}
	return st, r == 0
}

// triggerState applies on/off hysteresis: latch on at >=trigOn, off at <=trigOff, hold
// the previous state in the deadband between.
func triggerState(prev bool, v uint8) bool {
	if v >= trigOn {
		return true
	}
	if v <= trigOff {
		return false
	}
	return prev
}

// firstPad finds the lowest connected controller index, or -1.
func firstPad() int {
	for i := uint32(0); i < 4; i++ {
		if _, ok := getState(i); ok {
			return int(i)
		}
	}
	return -1
}

// sendKey presses (down=true) or releases a numpad key by scan code.
func sendKey(scan uint16, down bool) {
	flags := uint32(keyeventfScancode)
	if !down {
		flags |= keyeventfKeyUp
	}
	in := input{inputType: inputKeyboard, ki: keybdInput{wScan: scan, dwFlags: flags}}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func main() {
	if xiGetState == nil {
		fmt.Fprintln(os.Stderr, "vv-padbridge: no XInput DLL found")
		os.Exit(1)
	}
	if unsafe.Sizeof(input{}) != 40 {
		fmt.Fprintf(os.Stderr, "vv-padbridge: bad INPUT layout (%d)\n", unsafe.Sizeof(input{}))
		os.Exit(1)
	}

	pad := firstPad()
	if pad < 0 {
		fmt.Fprintln(os.Stderr, "vv-padbridge: no XInput pad connected — sticks/buttons still work natively")
		// Keep polling: a pad may connect after launch.
	}
	fmt.Fprintln(os.Stderr, "vv-padbridge: LT/LB->KP7  RT/RB->KP9  LB+RB->KP1(walk)")

	down := map[uint16]bool{scanKP7: false, scanKP9: false, scanKP1: false}
	hold := func(scan uint16, want bool) {
		if down[scan] != want {
			sendKey(scan, want)
			down[scan] = want
		}
	}

	// Sticky trigger state with hysteresis (on at >=trigOn, off at <=trigOff), so a
	// trigger resting in the deadband doesn't chatter — matches the evdev bridge.
	lt, rt := false, false

	ticker := time.NewTicker(8 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		idx := pad
		if idx < 0 {
			idx = firstPad()
			if idx < 0 {
				continue
			}
			pad = idx
		}
		st, ok := getState(uint32(idx))
		if !ok {
			pad = -1
			lt, rt = false, false
			hold(scanKP7, false)
			hold(scanKP9, false)
			hold(scanKP1, false)
			continue
		}
		g := st.Gamepad
		lt = triggerState(lt, g.LeftTrigger)
		rt = triggerState(rt, g.RightTrigger)
		lb := g.Buttons&xinputLeftShoulder != 0
		rb := g.Buttons&xinputRightShoulder != 0
		both := lb && rb

		hold(scanKP1, both)                // LB+RB held -> get off board
		hold(scanKP7, lt || (lb && !both)) // spin/rotate left
		hold(scanKP9, rt || (rb && !both)) // spin/rotate right
	}
}
