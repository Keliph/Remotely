# Blanket Remote

Control your PC (and soon your smart TV) from your phone.

| Folder | What it is |
|---|---|
| `pc/` | Windows app (Go). One `.exe`: serves the remote page, moves the mouse, answers the phone app. See `pc/README.md`. |
| `app/` | Phone app (Flutter, Android first). Finds PCs on the Wi-Fi, pairs with one tap, shows the remote. |
| `tv/` | `tv_test.py` – finds a smart TV on the Wi-Fi and controls it, to test TV support before it goes into the app. |

## Getting the APK and the exe

Every push to GitHub builds both automatically (`.github/workflows/build.yml`):

1. Push this folder to a GitHub repository (private is fine).
2. Open the repository → **Actions** → the latest **Build** run.
3. Under **Artifacts**, download `BlanketRemote-android` (the APK) and `BlanketRemote-windows` (the exe).

Install the APK on the phone (allow "install unknown apps" for your file manager/browser when Android asks).

The APK is signed with Flutter's debug key, which is fine for testing and sharing directly.
A Play Store release needs your own upload key; that's a later step.

## How the phone app finds the PC

No IP addresses or QR codes: the app broadcasts `BLANKETREMOTE?` on UDP port 47820, every
running PC answers with its id and port, and the app remembers PCs by id. First time, the PC
shows "Allow <phone>?" — after Yes, the phone keeps the key and connects instantly from then on.
