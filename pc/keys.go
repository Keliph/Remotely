package main

// Windows virtual-key codes for the key names the phone page sends.
// Kept platform-neutral so the names can be validated (and tested) anywhere.
var vkCodes = map[string]uint16{
	"backspace": 0x08, "tab": 0x09, "enter": 0x0D, "shift": 0x10, "ctrl": 0x11,
	"alt": 0x12, "esc": 0x1B, "space": 0x20, "pageup": 0x21, "pagedown": 0x22,
	"end": 0x23, "home": 0x24, "left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28,
	"insert": 0x2D, "delete": 0x2E, "win": 0x5B,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75,
	"f7": 0x76, "f8": 0x77, "f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
	"volumemute": 0xAD, "volumedown": 0xAE, "volumeup": 0xAF,
	"nexttrack": 0xB0, "prevtrack": 0xB1, "stop": 0xB2, "playpause": 0xB3,
}

// Keys that need the "extended" flag, otherwise Windows treats arrows etc. as numpad keys.
var extendedVK = map[uint16]bool{
	0x21: true, 0x22: true, 0x23: true, 0x24: true, 0x25: true, 0x26: true, 0x27: true,
	0x28: true, 0x2D: true, 0x2E: true, 0x5B: true,
	0xAD: true, 0xAE: true, 0xAF: true, 0xB0: true, 0xB1: true, 0xB2: true, 0xB3: true,
}

func init() {
	// single letters and digits: "a".."z", "0".."9"
	for c := 'a'; c <= 'z'; c++ {
		vkCodes[string(c)] = uint16(c - 'a' + 'A')
	}
	for c := '0'; c <= '9'; c++ {
		vkCodes[string(c)] = uint16(c)
	}
}

// Win32 INPUT structs. Plain Go with no build tag so their sizes can be unit-tested
// on any machine. SendInput rejects the call unless cbSize == sizeof(INPUT),
// which is 40 bytes on 64-bit Windows and 28 on 32-bit.
type mouseInput struct {
	Dx, Dy    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type keybdInput struct {
	Vk, Scan  uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
	_         [8]byte // pad to the size of MOUSEINPUT (largest union member)
}

type mouseINPUT struct {
	Type uint32
	Mi   mouseInput
}

type keyINPUT struct {
	Type uint32
	Ki   keybdInput
}
