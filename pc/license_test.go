package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Made with the seller's private key for the made-up PC code ABCD-EFGH.
const sampleCode = "BR-pbjSkJtjgm0LwdgBly1I1qMKQoWfHx8skT9C5n77rHhcglI0t7f16aPR43BqLPe1Ay_ZtCrSP9bNHJ-v8N5LDQ"

func TestUnlockCode(t *testing.T) {
	if !verifyUnlock("ABCD-EFGH", sampleCode) {
		t.Fatal("valid code rejected")
	}
	// pasted from WhatsApp with spaces / line breaks, or without the prefix
	messy := " " + sampleCode[:30] + "\n" + sampleCode[30:] + " "
	if !verifyUnlock("ABCD-EFGH", messy) || !verifyUnlock("ABCD-EFGH", strings.TrimPrefix(sampleCode, "BR-")) {
		t.Fatal("pasted code with spaces/line break rejected")
	}
	if verifyUnlock("ABCD-EFGJ", sampleCode) {
		t.Fatal("code for another PC accepted")
	}
	broken := []byte(sampleCode)
	broken[20] ^= 1
	if verifyUnlock("ABCD-EFGH", string(broken)) || verifyUnlock("ABCD-EFGH", "hello") {
		t.Fatal("bad code accepted")
	}
}

func TestPCCodeFormat(t *testing.T) {
	c := pcCode()
	if len(c) != 9 || c[4] != '-' {
		t.Fatalf("pc code %q", c)
	}
}

func setupLicenseTest(t *testing.T) string {
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "config.json")
	marker := filepath.Join(dir, "marker.json")
	t.Setenv("BR_TEST_MARKER", marker)
	cfgMu.Lock()
	cfg = config{ID: "test"}
	cfgMu.Unlock()
	return marker
}

func TestTrial(t *testing.T) {
	marker := setupLicenseTest(t)
	loadLicense()
	state, left := licenseState()
	if state != "trial" || left < 71*time.Hour {
		t.Fatalf("new PC: %s %v, want a 3-day trial", state, left)
	}

	// reinstall: config gone, marker kept -> same trial, not a new one
	var m licenseFile
	b, _ := os.ReadFile(marker)
	_ = json.Unmarshal(b, &m)
	m.Start -= int64((50 * time.Hour).Seconds())
	m.Sig = m.sign()
	b, _ = json.Marshal(m)
	_ = os.WriteFile(marker, b, 0o644)
	cfgMu.Lock()
	cfg.License = nil
	cfgMu.Unlock()
	loadLicense()
	if state, left = licenseState(); state != "trial" || left > 23*time.Hour {
		t.Fatalf("after reinstall: %s %v, want ~22h left", state, left)
	}

	// someone edits the start date by hand -> trial over
	m.Start = time.Now().Unix()
	b, _ = json.Marshal(m)
	_ = os.WriteFile(marker, b, 0o644)
	cfgMu.Lock()
	cfg.License = nil
	cfgMu.Unlock()
	loadLicense()
	if state, _ = licenseState(); state != "expired" {
		t.Fatalf("edited marker: %s, want expired", state)
	}
	if tryUnlock("nonsense") {
		t.Fatal("nonsense unlocked")
	}
}
