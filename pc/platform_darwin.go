//go:build darwin

package main

// macOS: mouse, keyboard, typing and media keys through CoreGraphics/AppKit.
// macOS only delivers these events once the user allows the app under
// System Settings > Privacy & Security > Accessibility.

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit -framework Foundation
#include <ApplicationServices/ApplicationServices.h>
#import <AppKit/AppKit.h>

static CGPoint br_cursor(void) {
	CGEventRef e = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	return p;
}

// Keep the pointer on a screen: if p is off every display, clamp it to the display the pointer is on.
static CGPoint br_clamp(CGPoint cur, CGPoint p) {
	CGDirectDisplayID ids[16];
	uint32_t n = 0;
	if (CGGetActiveDisplayList(16, ids, &n) != kCGErrorSuccess || n == 0) return p;
	for (uint32_t i = 0; i < n; i++) {
		if (CGRectContainsPoint(CGDisplayBounds(ids[i]), p)) return p;
	}
	CGRect r = CGDisplayBounds(ids[0]);
	for (uint32_t i = 0; i < n; i++) {
		CGRect b = CGDisplayBounds(ids[i]);
		if (CGRectContainsPoint(b, cur)) { r = b; break; }
	}
	if (p.x < CGRectGetMinX(r)) p.x = CGRectGetMinX(r);
	if (p.x > CGRectGetMaxX(r) - 1) p.x = CGRectGetMaxX(r) - 1;
	if (p.y < CGRectGetMinY(r)) p.y = CGRectGetMinY(r);
	if (p.y > CGRectGetMaxY(r) - 1) p.y = CGRectGetMaxY(r) - 1;
	return p;
}

static void br_move(double dx, double dy) {
	CGPoint cur = br_cursor();
	CGPoint p = br_clamp(cur, CGPointMake(cur.x + dx, cur.y + dy));
	CGEventRef e = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, p, kCGMouseButtonLeft);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void br_click(int right) {
	CGPoint p = br_cursor();
	CGEventType down = right ? kCGEventRightMouseDown : kCGEventLeftMouseDown;
	CGEventType up = right ? kCGEventRightMouseUp : kCGEventLeftMouseUp;
	CGMouseButton b = right ? kCGMouseButtonRight : kCGMouseButtonLeft;
	CGEventRef e1 = CGEventCreateMouseEvent(NULL, down, p, b);
	CGEventRef e2 = CGEventCreateMouseEvent(NULL, up, p, b);
	CGEventPost(kCGHIDEventTap, e1);
	CGEventPost(kCGHIDEventTap, e2);
	CFRelease(e1);
	CFRelease(e2);
}

static void br_scroll(int lines) {
	CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 1, lines);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void br_key(CGKeyCode code, CGEventFlags flags, int down) {
	CGEventRef e = CGEventCreateKeyboardEvent(NULL, code, down ? true : false);
	CGEventSetFlags(e, flags);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

// Types real characters (Urdu, emoji...) regardless of keyboard layout.
static void br_type(const UniChar *chars, int n) {
	for (int i = 0; i < n; i++) {
		int len = (i + 1 < n && CFStringIsSurrogateHighCharacter(chars[i])) ? 2 : 1;
		CGEventRef d = CGEventCreateKeyboardEvent(NULL, 0, true);
		CGEventRef u = CGEventCreateKeyboardEvent(NULL, 0, false);
		CGEventKeyboardSetUnicodeString(d, len, chars + i);
		CGEventKeyboardSetUnicodeString(u, len, chars + i);
		CGEventPost(kCGHIDEventTap, d);
		CGEventPost(kCGHIDEventTap, u);
		CFRelease(d);
		CFRelease(u);
		i += len - 1;
	}
}

// Play/pause, next, previous, volume: the same "system defined" events the Mac's own keys send.
static void br_media(int key) {
	@autoreleasepool {
		for (int down = 1; down >= 0; down--) {
			NSEvent *ev = [NSEvent otherEventWithType:NSEventTypeSystemDefined
			                                 location:NSZeroPoint
			                            modifierFlags:(down ? 0xa00 : 0xb00)
			                                timestamp:0
			                             windowNumber:0
			                                  context:nil
			                                  subtype:8
			                                    data1:((key << 16) | ((down ? 0xa : 0xb) << 8))
			                                    data2:-1];
			CGEventPost(kCGHIDEventTap, [ev CGEvent]);
		}
	}
}

// 1 if the user has allowed this app under Accessibility. prompt=1 shows macOS's own request dialog.
static int br_trusted(int prompt) {
	NSDictionary *opts = @{(__bridge id)kAXTrustedCheckOptionPrompt: prompt ? @YES : @NO};
	return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)opts) ? 1 : 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"
)

var inputMu sync.Mutex

// moveMouse: exact relative move (smoothing and acceleration happen in pointer.go).
func moveMouse(dx, dy int) {
	inputMu.Lock()
	defer inputMu.Unlock()
	C.br_move(C.double(dx), C.double(dy))
}

func wakeCursor()     {}
func precise(on bool) {}

func clickMouse(right bool) {
	r := 0
	if right {
		r = 1
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	C.br_click(C.int(r))
}

func scrollMouse(n int) {
	inputMu.Lock()
	defer inputMu.Unlock()
	C.br_scroll(C.int(n))
}

// Windows virtual-key codes (what keys.go uses) -> Mac key codes.
var macKeys = map[uint16]C.CGKeyCode{
	0x08: 0x33, 0x09: 0x30, 0x0D: 0x24, 0x1B: 0x35, 0x20: 0x31, // backspace tab enter esc space
	0x21: 0x74, 0x22: 0x79, 0x23: 0x77, 0x24: 0x73, // page up/down, end, home
	0x25: 0x7B, 0x26: 0x7E, 0x27: 0x7C, 0x28: 0x7D, // arrows
	0x2E: 0x75, // forward delete
	0x70: 0x7A, 0x71: 0x78, 0x72: 0x63, 0x73: 0x76, 0x74: 0x60, 0x75: 0x61,
	0x76: 0x62, 0x77: 0x64, 0x78: 0x65, 0x79: 0x6D, 0x7A: 0x67, 0x7B: 0x6F, // F1-F12
	'A': 0x00, 'S': 0x01, 'D': 0x02, 'F': 0x03, 'H': 0x04, 'G': 0x05, 'Z': 0x06, 'X': 0x07,
	'C': 0x08, 'V': 0x09, 'B': 0x0B, 'Q': 0x0C, 'W': 0x0D, 'E': 0x0E, 'R': 0x0F, 'Y': 0x10,
	'T': 0x11, 'O': 0x1F, 'U': 0x20, 'I': 0x22, 'P': 0x23, 'L': 0x25, 'J': 0x26, 'K': 0x28,
	'N': 0x2D, 'M': 0x2E,
	'1': 0x12, '2': 0x13, '3': 0x14, '4': 0x15, '6': 0x16, '5': 0x17, '9': 0x19, '7': 0x1A,
	'8': 0x1C, '0': 0x1D,
}

type macMod struct {
	code C.CGKeyCode
	flag C.CGEventFlags
}

var macMods = map[uint16]macMod{
	0x10: {0x38, 0x20000},  // shift
	0x11: {0x3B, 0x40000},  // control
	0x12: {0x3A, 0x80000},  // option (alt)
	0x5B: {0x37, 0x100000}, // command (the Windows key's spot)
}

// NX_KEYTYPE_* values for the media keys
var macMedia = map[uint16]int{0xAF: 0, 0xAE: 1, 0xAD: 7, 0xB3: 16, 0xB0: 17, 0xB1: 18}

// The phone sends Windows shortcuts; use what a Mac user expects instead.
func macShortcut(vks []uint16) []uint16 {
	const alt, ctrl, cmd, tab, f4, f11 = 0x12, 0x11, 0x5B, 0x09, 0x73, 0x7A
	switch {
	case len(vks) == 2 && vks[0] == alt && vks[1] == tab:
		return []uint16{cmd, tab} // switch app
	case len(vks) == 2 && vks[0] == alt && vks[1] == f4:
		return []uint16{cmd, 'Q'} // close app
	case len(vks) == 2 && vks[0] == ctrl && vks[1] >= 'A' && vks[1] <= 'Z':
		return []uint16{cmd, vks[1]} // select all, copy, paste...
	case len(vks) == 1 && vks[0] == f11:
		return []uint16{ctrl, cmd, 'F'} // full screen
	}
	return vks
}

func pressKeys(vks []uint16) {
	vks = macShortcut(vks)
	if len(vks) == 1 {
		if nx, ok := macMedia[vks[0]]; ok {
			inputMu.Lock()
			C.br_media(C.int(nx))
			inputMu.Unlock()
			return
		}
	}
	var flags C.CGEventFlags
	var mods, keys []C.CGKeyCode
	for _, vk := range vks {
		if m, ok := macMods[vk]; ok {
			flags |= m.flag
			mods = append(mods, m.code)
		} else if k, ok := macKeys[vk]; ok {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	for _, m := range mods {
		C.br_key(m, flags, 1)
	}
	for _, k := range keys {
		C.br_key(k, flags, 1)
	}
	for i := len(keys) - 1; i >= 0; i-- {
		C.br_key(keys[i], flags, 0)
	}
	for i := len(mods) - 1; i >= 0; i-- {
		C.br_key(mods[i], 0, 0)
	}
}

func typeText(s string) {
	inputMu.Lock()
	defer inputMu.Unlock()
	for i, line := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		if i > 0 {
			C.br_key(0x24, 0, 1) // Return
			C.br_key(0x24, 0, 0)
		}
		u := utf16.Encode([]rune(line))
		if len(u) > 0 {
			C.br_type((*C.UniChar)(unsafe.Pointer(&u[0])), C.int(len(u)))
		}
	}
}

func shutdownPC() error {
	return exec.Command("osascript", "-e", `tell application "System Events" to shut down`).Run()
}

func openBrowser(url string) { _ = exec.Command("open", url).Start() }

// askApprove shows a native "Allow?" dialog when a phone app asks to pair.
func askApprove(phone string) bool {
	phone = strings.NewReplacer(`"`, "", `\`, "").Replace(phone)
	dialog := fmt.Sprintf(`display dialog "Allow \"%s\" to control this Mac?\n\nOnly click Allow if this is your own phone." `+
		`with title "Blanket Remote" buttons {"Don't Allow", "Allow"} default button "Allow" `+
		`cancel button "Don't Allow" with icon caution giving up after 120`, phone)
	out, err := exec.Command("osascript", "-e", "tell me to activate", "-e", dialog).Output()
	return err == nil && strings.Contains(string(out), "button returned:Allow")
}

// displayName: the name people gave their Mac, e.g. "Epuh's MacBook Pro".
func displayName() string {
	if out, err := exec.Command("scutil", "--get", "ComputerName").Output(); err == nil {
		if n := strings.TrimSpace(string(out)); n != "" {
			return n
		}
	}
	h, _ := os.Hostname()
	return strings.TrimSuffix(h, ".local")
}

// ---- start at login: a per-user LaunchAgent ----

const agentLabel = "pk.blanketremote.agent"

func agentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist")
}

// macOS runs apps opened straight from Downloads from a temporary copy ("App Translocation").
// Starting at login would point at that temporary copy, so ask the user to move the app first.
func translocated() bool {
	exe, err := os.Executable()
	return err == nil && strings.Contains(exe, "/AppTranslocation/")
}

func setAutostart(on bool) error {
	p := agentPath()
	if !on {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if translocated() {
		return errors.New("move Blanket Remote to your Applications folder first")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(exe)
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + agentLabel + `</string>
	<key>ProgramArguments</key>
	<array><string>` + esc + `</string><string>--autostart</string></array>
	<key>RunAtLoad</key><true/>
	<key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(plist), 0o644)
}

func autostartEnabled() bool {
	_, err := os.Stat(agentPath())
	return err == nil
}

// ---- setup page hooks ----

func platformInit() {
	if C.br_trusted(1) == 0 { // shows macOS's "allow accessibility" request the first time
		log.Printf("accessibility permission not granted yet")
	}
}

func platformNotices() []map[string]string {
	var n []map[string]string
	if translocated() {
		n = append(n, map[string]string{
			"text": "Move Blanket Remote into your Applications folder and open it from there, so it can start with your Mac.",
		})
	}
	if C.br_trusted(0) == 0 {
		n = append(n, map[string]string{
			"text":   "Allow Blanket Remote to control this Mac: System Settings → Privacy & Security → Accessibility → turn on Blanket Remote. Until then the phone connects but the mouse won't move.",
			"action": "accessibility",
			"button": "Open settings",
		})
	}
	return n
}

func platformFix(what string) {
	if what == "accessibility" {
		C.br_trusted(1)
		_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility").Start()
	}
}
