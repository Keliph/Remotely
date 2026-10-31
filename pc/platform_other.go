//go:build !windows && !darwin

package main

// Stand-ins so the server can be run and tested on Linux/macOS.
// Each action is logged instead of moving a real mouse.

import (
	"log"
	"os"
)

func moveMouse(dx, dy int)   { log.Printf("INPUT move %d %d", dx, dy) }
func clickMouse(right bool)  { log.Printf("INPUT click right=%v", right) }
func scrollMouse(n int)      { log.Printf("INPUT scroll %d", n) }
func pressKeys(vks []uint16) { log.Printf("INPUT keys %v", vks) }
func typeText(s string)      { log.Printf("INPUT type %q", s) }
func shutdownPC() error      { log.Printf("INPUT shutdown"); return nil }
func openBrowser(url string) { log.Printf("OPEN %s", url) }

func askApprove(phone string) bool {
	log.Printf("PAIR request from %q", phone)
	return os.Getenv("BR_TEST_APPROVE") == "1"
}

var testAutostart bool

func setAutostart(on bool) error { testAutostart = on; return nil }
func autostartEnabled() bool     { return testAutostart }

func displayName() string {
	h, _ := os.Hostname()
	return h
}

func platformInit()                        {}
func platformNotices() []map[string]string { return nil }
func platformFix(what string)              {}

func mdnsName(id string) (string, bool) { return "blanketremote-" + shortID(id) + ".local", true }
