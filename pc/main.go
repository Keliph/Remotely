// Blanket Remote — control your PC from your phone.
// One .exe, no Python, no folder. Run it once; it starts with Windows after that.
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	appName       = "BlanketRemote"
	version       = "0.1.0"
	defaultPort   = 47801
	discoveryPort = 47820
	discoverAsk   = "BLANKETREMOTE?"
	discoverReply = "BLANKETREMOTE!"
)

// build is the commit this exe was built from (set by the cloud build), shown on the PC page so it's
// easy to see which version is running.
var build = "dev"

//go:embed web/remote.html web/setup.html web/fonts/*.woff2 web/apple-touch-icon.png
var webFS embed.FS

type config struct {
	Key  string `json:"key"`  // secret the phone needs (in the QR / given after pairing)
	ID   string `json:"id"`   // stable id so the phone app recognises this PC when its IP changes
	Port int    `json:"port"` // last port that worked
	// AutostartSet: start-at-login has been switched on once (or the user chose), so stop forcing it.
	AutostartSet bool `json:"autostart_set,omitempty"`
	// FirewallFixed: the one-time Windows Firewall fix has been applied.
	FirewallFixed bool `json:"firewall_fixed,omitempty"`
}

var (
	cfgMu    sync.Mutex
	cfg      config
	cfgPath  string
	hostname string
	port     int

	phones   atomic.Int32
	connsMu  sync.Mutex
	conns    = map[*websocket.Conn]bool{}
	pairMu   sync.Mutex
	lastDeny time.Time
)

func main() {
	autostartLaunch := len(os.Args) > 1 && os.Args[1] == "--autostart"

	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	dir = filepath.Join(dir, appName)
	_ = os.MkdirAll(dir, 0o700)
	cfgPath = filepath.Join(dir, "config.json")
	if f, err := os.Create(filepath.Join(dir, "log.txt")); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}

	firstRun := loadConfig()
	hostname = displayName()

	// Already running? Just show its page and quit (double-clicking the exe = "open settings").
	// Already running? Same version: just show its page (double-clicking the exe = "open settings").
	// Older or different version (e.g. you just downloaded an update): close it and take over.
	if other, running := runningBuild(cfg.Port); running {
		if other == build {
			openBrowser(setupURL(cfg.Port))
			return
		}
		log.Printf("replacing running build %q with %q", other, build)
		askToQuit(cfg.Port)
		for i := 0; i < 50; i++ {
			if _, still := runningBuild(cfg.Port); !still {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	ln, p := listen(cfg.Port)
	if ln == nil {
		log.Fatal("no free port")
	}
	port = p
	if cfg.Port != port {
		cfgMu.Lock()
		cfg.Port = port
		cfgMu.Unlock()
		saveConfig()
	}

	// Start with Windows: on by default, and refresh the path in case the exe was moved.
	// On by default until it has worked once (on a Mac it can't while the app runs from Downloads).
	cfgMu.Lock()
	wantAuto := firstRun || !cfg.AutostartSet
	cfgMu.Unlock()
	if wantAuto || autostartEnabled() {
		if err := setAutostart(true); err != nil {
			log.Printf("autostart: %v", err)
		} else if wantAuto {
			cfgMu.Lock()
			cfg.AutostartSet = true
			cfgMu.Unlock()
			saveConfig()
		}
	}

	platformInit()
	go discoveryResponder()

	mux := http.NewServeMux()
	mux.HandleFunc("/", servePage("web/remote.html", false))
	mux.HandleFunc("/fonts/", fontHandler)
	mux.HandleFunc("/apple-touch-icon.png", iconHandler)
	mux.HandleFunc("/apple-touch-icon-precomposed.png", iconHandler)
	mux.HandleFunc("/favicon.ico", iconHandler)
	mux.HandleFunc("/ws", wsHandler)
	mux.HandleFunc("/api/ping", pingHandler)
	mux.HandleFunc("/api/auth", authHandler)
	mux.HandleFunc("/api/pair", pairHandler)
	mux.HandleFunc("/setup", servePage("web/setup.html", true))
	mux.HandleFunc("/api/setup", localOnly(setupInfoHandler))
	mux.HandleFunc("/api/autostart", localOnly(autostartHandler))
	mux.HandleFunc("/api/newkey", localOnly(newKeyHandler))
	mux.HandleFunc("/api/quit", localOnly(quitHandler))
	mux.HandleFunc("/api/fix", localOnly(fixHandler))

	log.Printf("%s %s (build %s) on port %d, phone URL %s", appName, version, build, port, phoneURL())

	// nudge the mouse so Windows shows the cursor right away
	moveMouse(1, 0)
	moveMouse(-1, 0)

	if !autostartLaunch {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openBrowser(setupURL(port))
		}()
	}
	log.Fatal(http.Serve(ln, mux))
}

// ---------- config ----------

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func loadConfig() (firstRun bool) {
	b, err := os.ReadFile(cfgPath)
	if err != nil || json.Unmarshal(b, &cfg) != nil {
		firstRun = true
	}
	if len(cfg.Key) < 16 {
		cfg.Key = randHex(12)
	}
	if cfg.ID == "" {
		cfg.ID = randHex(6)
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	saveConfig()
	return firstRun
}

func saveConfig() {
	cfgMu.Lock()
	b, _ := json.MarshalIndent(cfg, "", "  ")
	cfgMu.Unlock()
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		log.Printf("save config: %v", err)
	}
}

func currentKey() string {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return cfg.Key
}

// ---------- startup helpers ----------

// runningBuild reports whether Blanket Remote already runs on this port, and which build it is
// ("" for versions from before builds were stamped).
func runningBuild(p int) (string, bool) {
	c := http.Client{Timeout: 700 * time.Millisecond}
	r, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", p))
	if err != nil {
		return "", false
	}
	defer r.Body.Close()
	var info struct {
		App   string `json:"app"`
		Build string `json:"build"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&info) != nil || info.App != appName {
		return "", false
	}
	return info.Build, true
}

func askToQuit(p int) {
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/api/quit", p), strings.NewReader("{}"))
	req.Header.Set("X-BR", "1")
	req.Header.Set("Content-Type", "application/json")
	c := http.Client{Timeout: 2 * time.Second}
	if r, err := c.Do(req); err == nil {
		r.Body.Close()
	}
}

func listen(preferred int) (net.Listener, int) {
	for p := preferred; p < preferred+30; p++ {
		if ln, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", p)); err == nil {
			return ln, p
		}
	}
	return nil, 0
}

func setupURL(p int) string { return fmt.Sprintf("http://127.0.0.1:%d/setup", p) }

// primaryIP returns the LAN address of the network card that has the default route.
func primaryIP() string {
	c, err := net.Dial("udp4", "8.8.8.8:80") // no packet is sent
	if err == nil {
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).IP.String()
	}
	if ips := lanIPs(); len(ips) > 0 {
		return ips[0]
	}
	return "127.0.0.1"
}

func lanIPs() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				if ip := n.IP.To4(); ip != nil && !ip.IsLinkLocalUnicast() {
					out = append(out, ip.String())
				}
			}
		}
	}
	return out
}

func phoneURL() string {
	return fmt.Sprintf("http://%s:%d/?key=%s", primaryIP(), port, currentKey())
}

// ---------- HTTP ----------

func servePage(name string, local bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/setup" {
			http.NotFound(w, r)
			return
		}
		if local && !isLocal(r) {
			http.Error(w, "Open this page on the PC itself.", http.StatusForbidden)
			return
		}
		b, _ := webFS.ReadFile(name)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	}
}

// iconHandler: the icon an iPhone uses for "Add to Home Screen".
func iconHandler(w http.ResponseWriter, r *http.Request) {
	b, _ := webFS.ReadFile("web/apple-touch-icon.png")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	_, _ = w.Write(b)
}

// fontHandler serves the bundled font so the phone page looks right even without internet.
func fontHandler(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	b, err := webFS.ReadFile("web/fonts/" + name)
	if err != nil || !strings.HasSuffix(name, ".woff2") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(b)
}

// isLocal: request comes from this PC *and* addresses it as localhost
// (the Host check blocks DNS-rebinding tricks from websites).
func isLocal(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return false
	}
	h := r.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	return h == "127.0.0.1" || h == "localhost"
}

// localOnly guards the settings API. POSTs must carry a custom header,
// which a web page on another site cannot add without a CORS preflight we never approve.
func localOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) || (r.Method == http.MethodPost && r.Header.Get("X-BR") != "1") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	cfgMu.Lock()
	id := cfg.ID
	cfgMu.Unlock()
	writeJSON(w, map[string]any{"app": appName, "v": version, "build": build, "id": id, "name": hostname, "os": runtime.GOOS})
}

// authHandler lets the phone app check in one request that a remembered address is still the
// same PC and that its saved key still works.
func authHandler(w http.ResponseWriter, r *http.Request) {
	ok := subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("key")), []byte(currentKey())) == 1
	cfgMu.Lock()
	id := cfg.ID
	cfgMu.Unlock()
	writeJSON(w, map[string]any{"app": appName, "id": id, "ok": ok, "name": hostname, "os": runtime.GOOS, "port": port})
}

func setupInfoHandler(w http.ResponseWriter, r *http.Request) {
	u := phoneURL()
	png, _ := qrcode.Encode(u, qrcode.Medium, 512)
	var others []string
	prim := primaryIP()
	for _, ip := range lanIPs() {
		if ip != prim {
			others = append(others, fmt.Sprintf("http://%s:%d/?key=%s", ip, port, currentKey()))
		}
	}
	writeJSON(w, map[string]any{
		"url":       u,
		"qr":        "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"others":    others,
		"name":      hostname,
		"phones":    phones.Load(),
		"autostart": autostartEnabled(),
		"version":   version,
		"build":     build,
		"os":        runtime.GOOS,
		"notices":   platformNotices(),
	})
}

func autostartHandler(w http.ResponseWriter, r *http.Request) {
	var body struct{ On bool }
	_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)
	if err := setAutostart(body.On); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	cfgMu.Lock()
	cfg.AutostartSet = true
	cfgMu.Unlock()
	saveConfig()
	writeJSON(w, map[string]any{"autostart": autostartEnabled()})
}

func newKeyHandler(w http.ResponseWriter, r *http.Request) {
	cfgMu.Lock()
	cfg.Key = randHex(12)
	cfgMu.Unlock()
	saveConfig()
	connsMu.Lock()
	for c := range conns {
		c.Close() // every phone has to scan/pair again
	}
	connsMu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

func fixHandler(w http.ResponseWriter, r *http.Request) {
	var body struct{ What string }
	_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)
	platformFix(body.What)
	writeJSON(w, map[string]any{"ok": true})
}

func quitHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
	go func() { time.Sleep(200 * time.Millisecond); os.Exit(0) }()
}

// pairHandler: the phone app found this PC by broadcast and asks for access.
// A Yes/No popup appears on the PC; on Yes the phone gets the key and keeps it.
func pairHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct{ Name string }
	_ = json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body)
	name := strings.Map(func(c rune) rune {
		if c < 32 || c == '"' {
			return -1
		}
		return c
	}, strings.TrimSpace(body.Name))
	if utf8.RuneCountInString(name) > 40 {
		name = string([]rune(name)[:40])
	}
	if name == "" {
		name = "A phone"
	}

	if !pairMu.TryLock() {
		http.Error(w, "Another phone is waiting for approval on the PC.", http.StatusTooManyRequests)
		return
	}
	defer pairMu.Unlock()
	if time.Since(lastDeny) < 10*time.Second {
		http.Error(w, "Denied recently, try again in a few seconds.", http.StatusTooManyRequests)
		return
	}
	log.Printf("pair request from %q (%s)", name, r.RemoteAddr)
	if !askApprove(name) {
		lastDeny = time.Now()
		http.Error(w, "Not allowed on the PC.", http.StatusForbidden)
		return
	}
	cfgMu.Lock()
	id := cfg.ID
	cfgMu.Unlock()
	writeJSON(w, map[string]any{"key": currentKey(), "id": id, "name": hostname, "port": port, "os": runtime.GOOS})
}

// ---------- WebSocket: the actual remote ----------

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // the key is the auth; the app's WebView has its own origin
}

type command struct {
	T    string   `json:"t"`
	DX   int      `json:"dx"`
	DY   int      `json:"dy"`
	B    string   `json:"b"`
	N    int      `json:"n"`
	Text string   `json:"text"`
	K    string   `json:"k"`
	Keys []string `json:"keys"`
	App  string   `json:"app"`
}

// Sites the Apps tab can open. Only these: the phone can't make the PC open anything else.
var appSites = map[string]string{
	"youtube":  "https://www.youtube.com",
	"netflix":  "https://www.netflix.com",
	"aryplus":  "https://aryplus.tv",
	"gmail":    "https://mail.google.com",
	"whatsapp": "https://web.whatsapp.com",
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	k := r.URL.Query().Get("key")
	if subtle.ConstantTimeCompare([]byte(k), []byte(currentKey())) != 1 {
		http.Error(w, "wrong key", http.StatusForbidden)
		return
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c.SetReadLimit(64 << 10)
	connsMu.Lock()
	conns[c] = true
	connsMu.Unlock()
	phones.Add(1)
	log.Printf("phone connected %s", r.RemoteAddr)
	defer func() {
		connsMu.Lock()
		delete(conns, c)
		connsMu.Unlock()
		phones.Add(-1)
		c.Close()
		log.Printf("phone disconnected %s", r.RemoteAddr)
	}()

	_ = c.WriteJSON(map[string]any{"t": "hello", "name": hostname, "v": version})

	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			return
		}
		var cmd command
		if json.Unmarshal(msg, &cmd) != nil {
			continue
		}
		handle(cmd)
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func handle(c command) {
	switch c.T {
	case "move":
		moveMouse(clamp(c.DX, -2000, 2000), clamp(c.DY, -2000, 2000))
	case "click":
		clickMouse(c.B == "right")
	case "scroll":
		scrollMouse(clamp(c.N, -50, 50))
	case "type":
		t := c.Text
		if utf8.RuneCountInString(t) > 2000 {
			t = string([]rune(t)[:2000])
		}
		typeText(t)
	case "key":
		if vk, ok := vkCodes[c.K]; ok {
			pressKeys([]uint16{vk})
		}
	case "hotkey":
		var vks []uint16
		for _, k := range c.Keys {
			if vk, ok := vkCodes[k]; ok {
				vks = append(vks, vk)
			}
		}
		if len(vks) > 0 && len(vks) <= 4 {
			pressKeys(vks)
		}
	case "open":
		if u, ok := appSites[c.App]; ok {
			openBrowser(u)
		}
	case "shutdown":
		if err := shutdownPC(); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}
}

// ---------- discovery: lets the phone app find this PC whatever its IP is ----------

func discoveryResponder() {
	pc, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", discoveryPort))
	if err != nil {
		log.Printf("discovery off: %v", err)
		return
	}
	defer pc.Close()
	buf := make([]byte, 256)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(string(buf[:n]), discoverAsk) {
			continue
		}
		cfgMu.Lock()
		id := cfg.ID
		cfgMu.Unlock()
		info, _ := json.Marshal(map[string]any{"id": id, "name": hostname, "port": port, "v": version, "os": runtime.GOOS})
		_, _ = pc.WriteTo(append([]byte(discoverReply), info...), addr)
	}
}
