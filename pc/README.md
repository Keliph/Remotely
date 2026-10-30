# Blanket Remote (PC side)

Control a Windows PC from your phone's browser. One `.exe`, no Python, no folder.

## Build
Needs Go 1.24+. From this folder:

    GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/BlanketRemote.exe .

`rsrc_windows_amd64.syso` (icon + manifest) is picked up automatically. To change the icon:

    go install github.com/akavel/rsrc@latest
    rsrc -manifest res/app.manifest -ico res/icon.ico -arch amd64 -o rsrc_windows_amd64.syso

`go run .` works on Linux/macOS too: input is only logged (platform_other.go), handy for testing the server.
`BR_TEST_APPROVE=1` auto-approves pairing there.

## Files
- `main.go` – config, HTTP + WebSocket server, setup page API, UDP discovery, pairing
- `platform_windows.go` – real mouse/keyboard (SendInput, Unicode typing), Yes/No popup, start-with-Windows
- `platform_darwin.go` – Mac: CoreGraphics mouse/keys/typing, media keys, Windows→Mac shortcut mapping, Allow dialog, LaunchAgent, Accessibility check (built on a macOS runner, needs cgo)
- `res/mac/` – Info.plist and icon for the Mac `.app`
- `keys.go` – key names → Windows key codes, INPUT structs (sizes pinned by `structs_test.go`)
- `web/remote.html` – phone UI (also loadable by a phone app: `remote.html?host=IP:PORT&key=KEY`)
- `web/setup.html` – PC page with QR code and settings (localhost only)

Settings + log live in `%APPDATA%\BlanketRemote\` (`config.json`, `log.txt`).

## Protocol for the phone app
- **Find PCs:** UDP broadcast `BLANKETREMOTE?` to port 47820. Each PC replies
  `BLANKETREMOTE!{"id":"…","name":"DESKTOP-…","port":47801,"v":"0.1.0"}`. Match PCs by `id`, not IP.
- **Pair once:** `POST http://IP:PORT/api/pair` body `{"name":"Samsung A15"}`. The PC shows a Yes/No popup;
  on Yes → `{"key":"…","id":"…","name":"…","port":…}`; on No → 403. Store the key per `id`.
- **Control:** WebSocket `ws://IP:PORT/ws?key=KEY`, JSON messages:
  `{"t":"move","dx":5,"dy":-3}` · `{"t":"click","b":"left|right"}` · `{"t":"scroll","n":2}` ·
  `{"t":"type","text":"…"}` · `{"t":"key","k":"playpause"}` · `{"t":"hotkey","keys":["alt","tab"]}` · `{"t":"shutdown"}`
- `GET /api/ping` → `{"app":"BlanketRemote","id":…}` to check a remembered address still is the same PC.
