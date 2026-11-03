package main

// Free trial and unlock codes.
//
// Every PC gets a 3-day free trial with everything working. After that the phone remote shows how to pay
// and asks for an unlock code. The buyer pays, sends their PC code (e.g. "7K3M-Q9PD") on WhatsApp, and the
// seller makes the unlock code for that PC with the private key (keygen page, never in this repo).
// Unlock codes are Ed25519 signatures: the app only holds the public key, so nothing in it can make codes.
//
// The trial start is kept in the config and in a second file outside the app's folders, so reinstalling
// doesn't restart the trial and a paid PC stays unlocked.

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

const trialLength = 72 * time.Hour

// unlockPublicKey: the Ed25519 public key (base64). Its private half is only in the seller's keygen page.
const unlockPublicKey = "ZbCm1yaYX0XRUsmGCDHTSez5tKMEYnAEVh6GYIEKBH0="

const unlockMessage = "blanket-remote-unlock-v1|"

// markerKey only makes the saved trial start awkward to edit by hand; it is not a secret.
var markerKey = []byte("blanket-remote-trial-marker-v1")

//go:embed web/pay.json
var payFS embed.FS

// payInfo: price and where to pay, shown on the phone and the PC page when the trial ends (web/pay.json).
var payInfo = func() json.RawMessage {
	b, err := payFS.ReadFile("web/pay.json")
	if err != nil || !json.Valid(b) {
		return json.RawMessage("{}")
	}
	return b
}()

// payQR: the Raast payment QR (bank apps, JazzCash, Easypaisa scan it) as an image for the PC page.
func payQR() string {
	var p struct {
		RaastQR string `json:"raast_qr"`
	}
	if json.Unmarshal(payInfo, &p) != nil || p.RaastQR == "" {
		return ""
	}
	png, err := qrcode.Encode(p.RaastQR, qrcode.Medium, 384)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

var crock = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

var (
	pcCodeOnce sync.Once
	pcCodeVal  string
)

// pcCode: short code for this computer, e.g. "7K3M-Q9PD". Stays the same when the app is reinstalled.
func pcCode() string {
	pcCodeOnce.Do(func() {
		id := machineID()
		if id == "" {
			cfgMu.Lock()
			id = "cfg:" + cfg.ID
			cfgMu.Unlock()
		}
		sum := sha256.Sum256([]byte("blanket-remote-pc|" + strings.ToLower(strings.TrimSpace(id))))
		s := crock.EncodeToString(sum[:5]) // 40 bits = 8 characters
		pcCodeVal = s[:4] + "-" + s[4:]
	})
	return pcCodeVal
}

// verifyUnlock: is code a valid unlock code for this PC code? Spaces, line breaks and a "BR-" prefix are fine.
func verifyUnlock(pc, code string) bool {
	code = strings.Join(strings.Fields(code), "")
	code = strings.TrimPrefix(strings.TrimPrefix(code, "BR-"), "br-")
	code = strings.TrimRight(code, "=")
	sig, err := base64.RawURLEncoding.DecodeString(code)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	pub, err := base64.StdEncoding.DecodeString(unlockPublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, []byte(unlockMessage+pc), sig)
}

// ---- trial state ----

type licenseFile struct {
	PC     string `json:"pc"`
	Start  int64  `json:"start"`
	Seen   int64  `json:"seen"`
	Unlock string `json:"unlock,omitempty"`
	Sig    string `json:"sig"`
}

func (l *licenseFile) sign() string {
	m := hmac.New(sha256.New, markerKey)
	m.Write([]byte(l.PC + "|" + time.Unix(l.Start, 0).UTC().Format(time.RFC3339)))
	return hex.EncodeToString(m.Sum(nil))
}

var (
	licMu     sync.Mutex
	lic       licenseFile
	licSaved  int64 // Seen value last written to disk
	licLoaded bool
)

func readMarker() (licenseFile, bool) {
	var l licenseFile
	p := licenseMarkerPath()
	if p == "" {
		return l, false
	}
	b, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(b, &l) != nil {
		return l, false
	}
	return l, true
}

// loadLicense merges the trial info from the config and the marker file (earliest start wins).
func loadLicense() {
	pc := pcCode()
	now := time.Now().Unix()
	var found []licenseFile
	if m, ok := readMarker(); ok {
		found = append(found, m)
	}
	cfgMu.Lock()
	if cfg.License != nil {
		found = append(found, *cfg.License)
	}
	cfgMu.Unlock()

	licMu.Lock()
	lic = licenseFile{PC: pc, Start: now, Seen: now}
	for _, f := range found {
		if f.Unlock != "" && verifyUnlock(pc, f.Unlock) {
			lic.Unlock = f.Unlock
		}
		if f.PC != pc {
			continue // copied from another PC
		}
		start := f.Start
		if f.Sig != f.sign() {
			start = now - int64(trialLength.Seconds()) // edited by hand: trial over
		}
		if start < lic.Start {
			lic.Start = start
		}
		if f.Seen > lic.Seen {
			lic.Seen = f.Seen
		}
	}
	licLoaded = true
	licMu.Unlock()
	saveLicense()

	go func() { // keep "last seen" fresh so setting the clock back doesn't extend the trial
		for range time.Tick(10 * time.Minute) {
			licenseState()
			saveLicense()
		}
	}()
}

func saveLicense() {
	licMu.Lock()
	l := lic
	l.Sig = l.sign()
	licSaved = l.Seen
	licMu.Unlock()

	cfgMu.Lock()
	cfg.License = &l
	cfgMu.Unlock()
	saveConfig()

	if p := licenseMarkerPath(); p != "" {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if b, err := json.Marshal(l); err == nil {
			if err := os.WriteFile(p, b, 0o644); err != nil {
				log.Printf("license marker: %v", err)
			}
		}
	}
}

// licenseState: "trial" (with time left), "expired" or "unlocked".
func licenseState() (string, time.Duration) {
	licMu.Lock()
	defer licMu.Unlock()
	if !licLoaded || lic.Unlock != "" {
		return "unlocked", 0
	}
	if now := time.Now().Unix(); now > lic.Seen {
		lic.Seen = now
	}
	left := trialLength - time.Duration(lic.Seen-lic.Start)*time.Second
	if left <= 0 {
		return "expired", 0
	}
	return "trial", left
}

func trialOver() bool {
	s, _ := licenseState()
	return s == "expired"
}

// tryUnlock saves code if it is valid for this PC, and tells every connected phone.
func tryUnlock(code string) bool {
	pc := pcCode()
	if !verifyUnlock(pc, code) {
		return false
	}
	licMu.Lock()
	lic.Unlock = strings.Join(strings.Fields(code), "")
	licMu.Unlock()
	saveLicense()
	log.Printf("unlocked")
	broadcast(map[string]any{"t": "license", "lic": licenseInfo()})
	return true
}

// licenseInfo goes to the phone (in "hello") and the PC page.
func licenseInfo() map[string]any {
	state, left := licenseState()
	return map[string]any{
		"state":      state,
		"hours_left": int(math.Ceil(left.Hours())),
		"pc":         pcCode(),
		"pay":        payInfo,
	}
}
