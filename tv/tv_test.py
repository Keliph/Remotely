"""
Blanket Remote - smart TV test.

Finds smart TVs on your Wi-Fi and lets you control one from this computer,
to prove the TV can be remote-controlled before we build it into the phone app.

    pip install androidtvremote2 websockets
    python tv_test.py              # scan the Wi-Fi
    python tv_test.py 192.168.1.20 # or go straight to a TV's IP

Android TV / Google TV (TCL, Haier, Xiaomi, Changhong, Sony, Android boxes): full test.
Samsung (Tizen): full test.  LG (webOS) and Roku: detected only, for now.
"""

import asyncio
import base64
import ipaddress
import json
import os
import socket
import ssl
import sys
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
APP_NAME = "Blanket Remote"

# port -> what usually listens there
PORTS = {
    6466: "android",   # Android TV Remote protocol v2 (what the Google TV phone app uses)
    8001: "samsung",   # Samsung remote WebSocket (plain)
    8002: "samsung",   # Samsung remote WebSocket (TLS, 2018+ models)
    3000: "lg",        # LG webOS SSAP
    3001: "lg",
    8060: "roku",      # Roku ECP
}

HELP = """
Commands (type one, press Enter):
  u d l r      up / down / left / right        ok     select
  back home    back / home                     menu   settings
  v+ v- mute   volume                          play   play / pause
  next prev    next / previous                 ff rw  fast-forward / rewind
  power        power toggle                    yt     open YouTube (Android TV)
  t <text>     type text into the focused box (e.g. "t cricket highlights")
  q            quit
"""

# ---------------------------------------------------------------- scanning


def local_ip():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(("8.8.8.8", 80))
        return s.getsockname()[0]
    except OSError:
        return "127.0.0.1"
    finally:
        s.close()


async def port_open(ip, port, timeout=0.6):
    try:
        _, w = await asyncio.wait_for(asyncio.open_connection(ip, port), timeout)
        w.close()
        return True
    except Exception:
        return False


def http_json(url, timeout=2):
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            return json.loads(r.read().decode("utf-8", "replace"))
    except Exception:
        return None


def http_text(url, timeout=2):
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            return r.read().decode("utf-8", "replace")
    except Exception:
        return ""


async def identify(ip, open_ports):
    kinds = {PORTS[p] for p in open_ports}
    if "android" in kinds:
        name = "Android TV"
        try:
            from androidtvremote2 import AndroidTVRemote
            cert, key = cert_paths()
            r = AndroidTVRemote(APP_NAME, cert, key, ip)
            await r.async_generate_cert_if_missing()
            n, _ = await asyncio.wait_for(r.async_get_name_and_mac(), 4)
            name = n or name
        except Exception:
            pass
        return {"ip": ip, "kind": "android", "name": name}
    if "samsung" in kinds:
        info = await asyncio.to_thread(http_json, f"http://{ip}:8001/api/v2/")
        dev = (info or {}).get("device", {})
        name = dev.get("name") or (info or {}).get("name") or "Samsung TV"
        model = dev.get("modelName", "")
        return {"ip": ip, "kind": "samsung", "name": f"{name} {model}".strip(),
                "tls": 8002 in open_ports, "plain": 8001 in open_ports}
    if "roku" in kinds:
        xml = await asyncio.to_thread(http_text, f"http://{ip}:8060/query/device-info")
        if "<device-info>" in xml:
            return {"ip": ip, "kind": "roku", "name": "Roku"}
    if "lg" in kinds:
        return {"ip": ip, "kind": "lg", "name": "LG webOS TV (probably)"}
    return None


async def scan():
    me = local_ip()
    net = ipaddress.ip_network(me + "/24", strict=False)
    print(f"Scanning {net} for smart TVs (this PC is {me})...")
    sem = asyncio.Semaphore(400)

    async def probe(ip, port):
        async with sem:
            return ip, port, await port_open(ip, port)

    hits = {}
    tasks = [probe(str(h), p) for h in net.hosts() if str(h) != me for p in PORTS]
    for ip, port, ok in await asyncio.gather(*tasks):
        if ok:
            hits.setdefault(ip, []).append(port)
    found = [d for d in await asyncio.gather(*(identify(ip, ps) for ip, ps in hits.items())) if d]
    return found


# ---------------------------------------------------------------- Android TV / Google TV


def cert_paths():
    return os.path.join(HERE, "tv_cert.pem"), os.path.join(HERE, "tv_key.pem")


ANDROID_KEYS = {
    "u": "DPAD_UP", "d": "DPAD_DOWN", "l": "DPAD_LEFT", "r": "DPAD_RIGHT", "ok": "DPAD_CENTER",
    "back": "BACK", "home": "HOME", "menu": "SETTINGS",
    "v+": "VOLUME_UP", "v-": "VOLUME_DOWN", "mute": "VOLUME_MUTE",
    "play": "MEDIA_PLAY_PAUSE", "next": "MEDIA_NEXT", "prev": "MEDIA_PREVIOUS",
    "ff": "MEDIA_FAST_FORWARD", "rw": "MEDIA_REWIND", "power": "POWER",
}


async def android_tv(dev):
    from androidtvremote2 import AndroidTVRemote, CannotConnect, ConnectionClosed, InvalidAuth

    cert, key = cert_paths()
    remote = AndroidTVRemote(APP_NAME, cert, key, dev["ip"])
    if await remote.async_generate_cert_if_missing():
        print("Made a new pairing certificate for this computer.")

    try:
        for attempt in range(3):
            try:
                await remote.async_connect()
                break
            except InvalidAuth:
                if attempt == 2:
                    raise
                print("\nPairing needed. A code will appear on the TV screen.")
                await remote.async_start_pairing()
                while True:
                    code = (await asyncio.to_thread(input, "Enter the code shown on the TV: ")).strip()
                    try:
                        await remote.async_finish_pairing(code)
                        print("Paired.")
                        break
                    except InvalidAuth:
                        print("Wrong code, try again.")
                    except ConnectionClosed:
                        print("TV closed pairing. Starting again...")
                        await remote.async_start_pairing()
    except (CannotConnect, ConnectionClosed, InvalidAuth) as e:
        print(f"Can't connect to the TV ({type(e).__name__}). Is it an Android TV / Google TV, switched on, on the same Wi-Fi?")
        return

    info = remote.device_info or {}
    print(f"\nConnected to {info.get('manufacturer', '')} {info.get('model', dev['name'])}".rstrip())
    remote.keep_reconnecting()
    print(HELP)
    while True:
        cmd = (await asyncio.to_thread(input, "tv> ")).strip()
        if cmd in ("q", "quit", "exit"):
            break
        try:
            if cmd.startswith("t "):
                remote.send_text(cmd[2:])
            elif cmd == "yt":
                remote.send_launch_app_command("https://www.youtube.com")
            elif cmd in ANDROID_KEYS:
                remote.send_key_command(ANDROID_KEYS[cmd])
            elif cmd:
                print("Unknown command. Type one from the list above.")
        except ConnectionClosed:
            print("Lost connection, reconnecting...")
    remote.disconnect()


# ---------------------------------------------------------------- Samsung (Tizen)

SAMSUNG_KEYS = {
    "u": "KEY_UP", "d": "KEY_DOWN", "l": "KEY_LEFT", "r": "KEY_RIGHT", "ok": "KEY_ENTER",
    "back": "KEY_RETURN", "home": "KEY_HOME", "menu": "KEY_MENU",
    "v+": "KEY_VOLUP", "v-": "KEY_VOLDOWN", "mute": "KEY_MUTE",
    "play": "KEY_PLAY_BACK", "next": "KEY_FF", "prev": "KEY_REWIND",
    "ff": "KEY_FF", "rw": "KEY_REWIND", "power": "KEY_POWER",
}


def samsung_token_path(ip):
    return os.path.join(HERE, f"samsung_token_{ip.replace('.', '_')}.txt")


async def samsung_tv(dev):
    import websockets

    name64 = base64.b64encode(APP_NAME.encode()).decode()
    tok_file = samsung_token_path(dev["ip"])
    token = open(tok_file).read().strip() if os.path.exists(tok_file) else ""
    if dev.get("tls"):
        # The TV uses its own self-signed certificate, so it can't be verified like a website's.
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        url = f"wss://{dev['ip']}:8002/api/v2/channels/samsung.remote.control?name={name64}"
        if token:
            url += f"&token={token}"
    else:
        ctx = None
        url = f"ws://{dev['ip']}:8001/api/v2/channels/samsung.remote.control?name={name64}"

    print("\nConnecting. If the TV asks to allow \"Blanket Remote\", choose Allow on the TV.")
    try:
        ws = await websockets.connect(url, ssl=ctx, open_timeout=10, max_size=2**22)
    except Exception as e:
        print(f"Can't connect to the TV: {e}")
        return
    async with ws:
        try:
            while True:
                msg = json.loads(await asyncio.wait_for(ws.recv(), 40))
                if msg.get("event") == "ms.channel.connect":
                    t = (msg.get("data") or {}).get("token")
                    if t:
                        open(tok_file, "w").write(t)
                    break
                if msg.get("event") == "ms.channel.unauthorized":
                    print("The TV said no (Deny was chosen, or remote access is off in TV settings).")
                    return
        except asyncio.TimeoutError:
            print("No answer from the TV. Check the TV screen for an Allow popup and try again.")
            return

        print(f"\nConnected to {dev['name']}")
        print(HELP)
        while True:
            cmd = (await asyncio.to_thread(input, "tv> ")).strip()
            if cmd in ("q", "quit", "exit"):
                break
            if cmd.startswith("t "):
                payload = {"method": "ms.remote.control", "params": {
                    "Cmd": base64.b64encode(cmd[2:].encode()).decode(), "DataOfCmd": "base64.utf8",
                    "TypeOfRemote": "SendInputString"}}
            elif cmd in SAMSUNG_KEYS:
                payload = {"method": "ms.remote.control", "params": {
                    "Cmd": "Click", "DataOfCmd": SAMSUNG_KEYS[cmd], "Option": "false",
                    "TypeOfRemote": "SendRemoteKey"}}
            elif cmd == "yt":
                print("App launch on Samsung comes later.")
                continue
            else:
                if cmd:
                    print("Unknown command. Type one from the list above.")
                continue
            try:
                await ws.send(json.dumps(payload))
            except Exception as e:
                print(f"Lost connection: {e}")
                return


# ---------------------------------------------------------------- main


async def main():
    if len(sys.argv) > 1:
        ip = sys.argv[1]
        ports = [p for p in PORTS if await port_open(ip, p, 1.5)]
        dev = await identify(ip, ports) if ports else None
        found = [dev] if dev else []
        if not found:
            print(f"No smart-TV remote service answered at {ip} (checked ports {', '.join(map(str, PORTS))}).")
    else:
        found = await scan()

    if not found:
        print("\nNo smart TV found. Check that the TV is on (not standby) and on the same Wi-Fi as this computer.")
        return

    print("\nFound:")
    for i, d in enumerate(found, 1):
        print(f"  {i}. {d['name']}  [{d['kind']}]  {d['ip']}")
    pick = 0
    if len(found) > 1:
        pick = int((await asyncio.to_thread(input, "Which one? ")).strip() or 1) - 1
    dev = found[pick]

    if dev["kind"] == "android":
        await android_tv(dev)
    elif dev["kind"] == "samsung":
        await samsung_tv(dev)
    else:
        print(f"\nFound a {dev['kind']} TV. Control for this brand isn't in the test yet. Tell Claude the brand and model.")


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
