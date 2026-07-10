package core

import "testing"

// Sample dinput_probe output (the GAMECTRL pass finds the pad; the ALL pass adds
// mouse/keyboard which must NOT be picked).
const probeOut = `DirectInput8Create hr=0x00000000 di=0000ABCD

=== EnumDevices(DI8DEVCLASS_GAMECTRL, ATTACHEDONLY) ===
  DEVICE: "Controller (XBOX 360 For Windows)" (product "Controller (XBOX 360 For Windows)")
          devType=0x00010215 -> GAMEPAD (subtype 21) [HID]
          guidInstance=A1B2C3D4-1234-11EF-8001-444553540000
          guidProduct =028E045E-0000-0000-0000-504944564944
  (EnumDevices GAMECTRL hr=0x00000000)

=== EnumDevices(DI8DEVCLASS_ALL, ATTACHEDONLY) ===
  DEVICE: "Mouse" (product "Mouse")
          devType=0x00000112 -> MOUSE (subtype 1)
          guidInstance=6F1D2B60-D5A0-11CF-BFC7-444553540000
          guidProduct =6F1D2B60-D5A0-11CF-BFC7-444553540000
  (EnumDevices ALL hr=0x00000000)

--- done ---`

func TestParseGamepadGUID(t *testing.T) {
	got := parseGamepadGUID(probeOut)
	want := "A1B2C3D4-1234-11EF-8001-444553540000"
	if got != want {
		t.Errorf("parseGamepadGUID = %q, want the GAMEPAD's GUID %q", got, want)
	}
}

// realProbeOut is the verbatim dinput_probe_guid.exe output captured on the user's
// Windows laptop (Xbox 360 wired pad) the day this lane's controller was solved. The
// GAMECTRL pass finds the pad; the ALL pass then lists mouse+keyboard+pad — the parser
// must return the pad's guidInstance, never a keyboard/mouse.
const realProbeOut = `DirectInput8Create hr=0x00000000 di=01067d8c

=== EnumDevices(DI8DEVCLASS_GAMECTRL, ATTACHEDONLY) ===
  DEVICE: "Controller (XBOX 360 For Windows)" (product "Controller (XBOX 360 For Windows)")
          devType=0x00010215 -> GAMEPAD (subtype 2) [HID]
          guidInstance=1EF49C40-7B0B-11F1-8002-444553540000
          guidProduct =028E045E-0000-0000-0000-504944564944
  (EnumDevices GAMECTRL hr=0x00000000)

=== EnumDevices(DI8DEVCLASS_ALL, ATTACHEDONLY) ===
  DEVICE: "Mouse" (product "Mouse")
          devType=0x00000112 -> MOUSE (subtype 1)
          guidInstance=6F1D2B60-D5A0-11CF-BFC7-444553540000
          guidProduct =6F1D2B60-D5A0-11CF-BFC7-444553540000
  DEVICE: "Keyboard" (product "Keyboard")
          devType=0x00000413 -> KEYBOARD (subtype 4)
          guidInstance=6F1D2B61-D5A0-11CF-BFC7-444553540000
          guidProduct =6F1D2B61-D5A0-11CF-BFC7-444553540000
  DEVICE: "Controller (XBOX 360 For Windows)" (product "Controller (XBOX 360 For Windows)")
          devType=0x00010215 -> GAMEPAD (subtype 2) [HID]
          guidInstance=1EF49C40-7B0B-11F1-8002-444553540000
          guidProduct =028E045E-0000-0000-0000-504944564944
  (EnumDevices ALL hr=0x00000000)

--- done ---`

func TestParseGamepadGUID_RealCapture(t *testing.T) {
	got := parseGamepadGUID(realProbeOut)
	want := "1EF49C40-7B0B-11F1-8002-444553540000"
	if got != want {
		t.Errorf("parseGamepadGUID(real) = %q, want the Xbox 360 pad GUID %q (not the mouse/keyboard)", got, want)
	}
}

func TestParseGamepadGUID_JoystickFallback(t *testing.T) {
	out := `=== EnumDevices(DI8DEVCLASS_GAMECTRL, ATTACHEDONLY) ===
  DEVICE: "Some Stick"
          devType=0x00010114 -> JOYSTICK (subtype 1) [HID]
          guidInstance=DEADBEEF-0000-0000-0000-000000000001
`
	if got := parseGamepadGUID(out); got != "DEADBEEF-0000-0000-0000-000000000001" {
		t.Errorf("JOYSTICK GUID not parsed, got %q", got)
	}
}

func TestParseGamepadGUID_None(t *testing.T) {
	if got := parseGamepadGUID("no devices here\n"); got != "" {
		t.Errorf("expected empty for no devices, got %q", got)
	}
}
