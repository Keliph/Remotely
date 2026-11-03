package main

// "Update available" notice.
//
// Every release publishes version.json next to the downloads. The PC app compares it with its own version
// (the time of the last change to the PC app, stamped in by the build) and, when a newer one is out, shows
// a notice on the PC page and on the phone. "Update" opens the download in the PC's browser; opening the
// downloaded file replaces the running version (see runningBuild in main.go) and keeps phones and unlock.
// The app deliberately doesn't download and swap its own exe: antivirus programs flag apps that do that.
// The Microsoft Store version is updated by the Store, so it doesn't check.

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pcVersion: set by the build (-X main.pcVersion=<unix time of the last change to pc/>). Empty in dev builds.
var pcVersion = ""

const versionURL = "https://github.com/Keliph/Remotely/releases/download/latest/version.json"
const downloadPrefix = "https://github.com/Keliph/Remotely/releases/"

type releaseInfo struct {
	PC    int64  `json:"pc"`
	App   int64  `json:"app"`
	Build string `json:"build"`
	Exe   string `json:"exe"`
	Mac   string `json:"mac"`
	Apk   string `json:"apk"`
}

var (
	updMu     sync.Mutex
	updNewer  *releaseInfo // a newer release, or nil
	updOpened bool         // the download was opened on this PC
)

func myVersion() int64 {
	v, _ := strconv.ParseInt(pcVersion, 10, 64)
	return v
}

// downloadLink: this computer's download from the release.
func (r *releaseInfo) downloadLink() string {
	link := r.Exe
	if runtime.GOOS == "darwin" {
		link = r.Mac
	}
	if !strings.HasPrefix(link, downloadPrefix) {
		return ""
	}
	return link
}

func updateChecker() {
	if myVersion() == 0 || isStoreVersion() {
		return
	}
	time.Sleep(15 * time.Second)
	for {
		checkForUpdate()
		time.Sleep(6 * time.Hour)
	}
}

func checkForUpdate() {
	url := versionURL
	if u := os.Getenv("BR_TEST_VERSION_URL"); u != "" {
		url = u
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var r releaseInfo
	if json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&r) != nil || r.PC <= myVersion() || r.downloadLink() == "" {
		return
	}
	updMu.Lock()
	known := updNewer != nil && updNewer.PC == r.PC
	updNewer = &r
	updMu.Unlock()
	if !known {
		log.Printf("update available: build %s", r.Build)
		broadcast(map[string]any{"t": "update", "upd": updateInfo()})
	}
}

// updateInfo goes to the phone (in "hello") and the PC page; nil when there's no update.
func updateInfo() map[string]any {
	updMu.Lock()
	defer updMu.Unlock()
	if updNewer == nil {
		return nil
	}
	return map[string]any{"link": updNewer.downloadLink(), "opened": updOpened, "os": runtime.GOOS}
}

// openUpdate: the phone's "Update" button. Opens the download in this PC's browser.
func openUpdate() {
	updMu.Lock()
	r := updNewer
	if r != nil {
		updOpened = true
	}
	updMu.Unlock()
	if r == nil {
		return
	}
	openBrowser(r.downloadLink())
	broadcast(map[string]any{"t": "update", "upd": updateInfo()})
}
