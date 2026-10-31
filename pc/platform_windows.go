//go:build windows

package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	advapi32          = syscall.NewLazyDLL("advapi32.dll")
	procSendInput     = user32.NewProc("SendInput")
	procMapVirtual    = user32.NewProc("MapVirtualKeyW")
	procMessageBox    = user32.NewProc("MessageBoxW")
	procGetCursorPos  = user32.NewProc("GetCursorPos")
	procSetCursorPos  = user32.NewProc("SetCursorPos")
	procTimePeriodOn  = syscall.NewLazyDLL("winmm.dll").NewProc("timeBeginPeriod")
	procTimePeriodOff = syscall.NewLazyDLL("winmm.dll").NewProc("timeEndPeriod")
	procRegSetValue   = advapi32.NewProc("RegSetValueExW")
	procRegDelValue   = advapi32.NewProc("RegDeleteValueW")
	procRegCreateKey  = advapi32.NewProc("RegCreateKeyExW")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mMove      = 0x0001
	mLeftDown  = 0x0002
	mLeftUp    = 0x0004
	mRightDown = 0x0008
	mRightUp   = 0x0010
	mWheel     = 0x0800

	kExtended = 0x0001
	kUp       = 0x0002
	kUnicode  = 0x0004
)

var inputMu sync.Mutex

func sendMouse(ins ...mouseINPUT) {
	if len(ins) == 0 {
		return
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	procSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
}

func sendKeys(ins []keyINPUT) {
	if len(ins) == 0 {
		return
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	procSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
}

func mouse(flags uint32, dx, dy int32, data uint32) mouseINPUT {
	return mouseINPUT{Type: inputMouse, Mi: mouseInput{Dx: dx, Dy: dy, MouseData: data, Flags: flags}}
}

func vkEvent(vk uint16, up bool) keyINPUT {
	scan, _, _ := procMapVirtual.Call(uintptr(vk), 0) // MAPVK_VK_TO_VSC
	var f uint32
	if extendedVK[vk] {
		f |= kExtended
	}
	if up {
		f |= kUp
	}
	return keyINPUT{Type: inputKeyboard, Ki: keybdInput{Vk: vk, Scan: uint16(scan), Flags: f}}
}

// moveMouse places the cursor exactly dx, dy pixels further. Setting the position directly (instead of
// sending a relative mouse move) skips Windows' "enhance pointer precision" acceleration, which made
// the small smooth steps uneven; the app's own curve (pointer.go) is used instead.
func moveMouse(dx, dy int) {
	var pt struct{ X, Y int32 }
	inputMu.Lock()
	defer inputMu.Unlock()
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r == 0 {
		procSendInput.Call(1, uintptr(unsafe.Pointer(&[]mouseINPUT{mouse(mMove, int32(dx), int32(dy), 0)}[0])), unsafe.Sizeof(mouseINPUT{}))
		return
	}
	procSetCursorPos.Call(uintptr(pt.X+int32(dx)), uintptr(pt.Y+int32(dy)))
}

// wakeCursor sends a real (tiny, cancelling) mouse move, so Windows shows the cursor even on touch
// laptops where it hides it until a mouse is used.
func wakeCursor() { sendMouse(mouse(mMove, 1, 0, 0), mouse(mMove, -1, 0, 0)) }

// precise asks Windows for 1 ms timer accuracy while the cursor is moving (smooth 4 ms steps).
func precise(on bool) {
	if on {
		procTimePeriodOn.Call(1)
	} else {
		procTimePeriodOff.Call(1)
	}
}

func clickMouse(right bool) {
	if right {
		sendMouse(mouse(mRightDown, 0, 0, 0), mouse(mRightUp, 0, 0, 0))
	} else {
		sendMouse(mouse(mLeftDown, 0, 0, 0), mouse(mLeftUp, 0, 0, 0))
	}
}

func scrollMouse(notches int) {
	sendMouse(mouse(mWheel, 0, 0, uint32(int32(notches*120))))
}

func pressKeys(vks []uint16) {
	// hold each key down in order, release in reverse: alt+tab, ctrl+c, etc.
	var ins []keyINPUT
	for _, vk := range vks {
		ins = append(ins, vkEvent(vk, false))
	}
	for i := len(vks) - 1; i >= 0; i-- {
		ins = append(ins, vkEvent(vks[i], true))
	}
	sendKeys(ins)
}

// typeText sends real Unicode characters, so Urdu, emoji etc. work
// (pyautogui.write could only type English keyboard characters).
func typeText(s string) {
	var ins []keyINPUT
	for _, u := range utf16.Encode([]rune(s)) {
		switch u {
		case '\r':
			continue
		case '\n':
			ins = append(ins, vkEvent(0x0D, false), vkEvent(0x0D, true))
		default:
			ins = append(ins,
				keyINPUT{Type: inputKeyboard, Ki: keybdInput{Scan: u, Flags: kUnicode}},
				keyINPUT{Type: inputKeyboard, Ki: keybdInput{Scan: u, Flags: kUnicode | kUp}})
		}
	}
	sendKeys(ins)
}

func hidden(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return c
}

func shutdownPC() error { return hidden("shutdown", "/s", "/t", "5").Run() }

func openBrowser(url string) { _ = hidden("rundll32", "url.dll,FileProtocolHandler", url).Start() }

// askApprove shows a Yes/No popup on the PC when a phone app asks to pair.
func askApprove(phone string) bool {
	msg := fmt.Sprintf("Allow \"%s\" to control this PC?\n\nOnly click Yes if this is your own phone.", phone)
	m, _ := syscall.UTF16PtrFromString(msg)
	t, _ := syscall.UTF16PtrFromString("Blanket Remote")
	const flags = 0x4 | 0x20 | 0x10000 | 0x40000 // YESNO | ICONQUESTION | SETFOREGROUND | TOPMOST
	r, _, _ := procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return r == 6 // IDYES
}

// ---- start with Windows (per-user Run key, no admin needed) ----

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func openRunKey() (syscall.Handle, error) {
	var h syscall.Handle
	p, _ := syscall.UTF16PtrFromString(runKeyPath)
	const keyAllAccess = 0xF003F
	r, _, _ := procRegCreateKey.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(p)),
		0, 0, 0, keyAllAccess, 0, uintptr(unsafe.Pointer(&h)), 0)
	if r != 0 {
		return 0, syscall.Errno(r)
	}
	return h, nil
}

func setAutostart(on bool) error {
	h, err := openRunKey()
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(h)
	name, _ := syscall.UTF16PtrFromString(appName)
	if !on {
		r, _, _ := procRegDelValue.Call(uintptr(h), uintptr(unsafe.Pointer(name)))
		if r != 0 && r != 2 { // 2 = already absent
			return syscall.Errno(r)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	data := utf16.Encode([]rune(`"` + exe + `" --autostart` + "\x00"))
	r, _, _ := procRegSetValue.Call(uintptr(h), uintptr(unsafe.Pointer(name)), 0, syscall.REG_SZ,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func autostartEnabled() bool {
	h, err := openRunKey()
	if err != nil {
		return false
	}
	defer syscall.RegCloseKey(h)
	name, _ := syscall.UTF16PtrFromString(appName)
	var typ, n uint32
	return syscall.RegQueryValueEx(h, name, nil, &typ, nil, &n) == nil
}

func displayName() string {
	h, _ := os.Hostname()
	return h
}

// ---- Windows Firewall ----
// Windows often marks a wired (Ethernet) connection as a "Public" network. The firewall popup only
// allows "Private" by default, so on a Public network the phone is silently blocked.

var onPublicNet atomic.Bool

func platformInit() {
	go func() {
		for {
			out, err := hidden("powershell", "-NoProfile", "-NonInteractive", "-Command",
				"(Get-NetConnectionProfile).NetworkCategory").Output()
			onPublicNet.Store(err == nil && strings.Contains(string(out), "Public"))
			time.Sleep(30 * time.Second)
		}
	}()
}

func platformNotices() []map[string]string {
	cfgMu.Lock()
	fixed := cfg.FirewallFixed
	cfgMu.Unlock()
	if fixed || !onPublicNet.Load() {
		return nil
	}
	return []map[string]string{{
		"text":   "Windows has this PC's network set to Public, so the firewall blocks your phone. Click Fix — Windows will ask for permission once.",
		"action": "firewall",
		"button": "Fix",
	}}
}

// platformFix("firewall") removes any rule Windows made when the popup was dismissed (those block the app)
// and allows Blanket Remote on every network type. Needs admin, so it asks through one UAC prompt.
func platformFix(what string) {
	if what != "firewall" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	script := "$exe = '" + strings.ReplaceAll(exe, "'", "''") + "'\n" +
		"Get-NetFirewallApplicationFilter -ErrorAction SilentlyContinue | Where-Object { $_.Program -ieq $exe } | " +
		"Get-NetFirewallRule -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue\n" +
		"Get-NetFirewallRule -DisplayName 'Blanket Remote' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue\n" +
		"New-NetFirewallRule -DisplayName 'Blanket Remote' -Direction Inbound -Action Allow -Program $exe -Profile Any | Out-Null\n"
	outer := "Start-Process powershell -Verb RunAs -Wait -WindowStyle Hidden " +
		"-ArgumentList '-NoProfile','-NonInteractive','-EncodedCommand','" + psEncode(script) + "'"
	if err := hidden("powershell", "-NoProfile", "-NonInteractive", "-Command", outer).Run(); err != nil {
		log.Printf("firewall fix not applied: %v", err) // e.g. the UAC prompt was declined
		return
	}
	cfgMu.Lock()
	cfg.FirewallFixed = true
	cfgMu.Unlock()
	saveConfig()
	log.Printf("firewall rule added for all network types")
}

// psEncode makes a PowerShell -EncodedCommand argument (base64 of UTF-16LE), which avoids all quoting issues.
func psEncode(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[2*i] = byte(c)
		b[2*i+1] = byte(c >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}
