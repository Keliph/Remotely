"""Draws the Blanket Remote icon (a phone peeking out of a blanket) and writes every icon file:
Windows (.ico + .png), Mac (.iconset) and Android (classic + adaptive launcher icons).

    python3 tool/make_icons.py            # then rebuild the Windows resource, see pc/README.md
"""
import os, pathlib, sys
from PIL import Image, ImageDraw

ROOT = pathlib.Path(__file__).resolve().parent.parent

# colours, matched to the remote's amber glass look
TILE = (34, 24, 20, 255)          # warm near-black, like the remote's background
BACK = (214, 70, 0, 255)          # blanket fold behind the phone (deep red-orange)
FRONT = (255, 122, 0, 255)        # blanket fold in front (bright orange)
PHONE = (246, 238, 228, 255)      # phone body, warm white
SCREEN = TILE
DOT = (255, 160, 30, 255)         # "connected" light on the screen


def scene(d, k, ox, oy, width, layer, c):
    """Artwork in a 1024-unit design space, scaled by k and shifted by (ox, oy). The blanket runs across
    the full canvas width. layer: 'back', 'front' or 'all'."""
    P = lambda x, y: (ox + x * k, oy + y * k)

    def fold(top, wave_top, wave_bot, start, fill):
        d.rectangle([0, P(0, top)[1], width, width * 2], fill=fill)
        x = start
        while P(x, 0)[0] > -200 * k:
            x -= 180
        while P(x, 0)[0] < width + 10:
            (x0, y0), (x1, y1) = P(x, wave_top), P(x + 200, wave_bot)
            d.ellipse([x0, y0, x1, y1], fill=fill)
            x += 180

    def box(a, b, cc, e, **kw):
        (x0, y0), (x1, y1) = P(a, b), P(cc, e)
        return [x0, y0, x1, y1]

    if layer in ("back", "all"):
        fold(660, 610, 710, -60, c["back"])
    if layer in ("front", "all"):
        d.rounded_rectangle(box(352, 170, 672, 820), radius=60 * k, fill=c["phone"])
        d.rounded_rectangle(box(382, 230, 642, 820), radius=26 * k, fill=c["screen"])
        d.ellipse(box(472, 370, 552, 450), fill=c["dot"])
        d.ellipse(box(442, 340, 582, 480), outline=c["dot"], width=max(1, round(14 * k)))
        fold(730, 680, 790, 30, c["front"])


def tile_icon(size, c, ss=4):
    """The rounded-square icon (Windows, Mac, older Android phones)."""
    S = size * ss
    k = S / 1024
    img = Image.new("RGBA", (S, S), c["tile"])
    scene(ImageDraw.Draw(img), k, 0, 0, S, "all", c)
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([40 * k, 40 * k, S - 40 * k, S - 40 * k], radius=220 * k, fill=255)
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    out.paste(img, (0, 0), mask)
    return out.resize((size, size), Image.LANCZOS)


def adaptive_layers(px, c, ss=4):
    """Android 8+ adaptive icon: 108dp canvas, artwork in the middle 72dp so every phone shape fits it."""
    big = px * ss
    k = (big * 72 / 108) / 1024
    off = big * 18 / 108
    bg = Image.new("RGBA", (big, big), c["tile"])
    scene(ImageDraw.Draw(bg), k, off, off, big, "back", c)
    fg = Image.new("RGBA", (big, big), (0, 0, 0, 0))
    scene(ImageDraw.Draw(fg), k, off, off, big, "front", c)
    return bg.resize((px, px), Image.LANCZOS), fg.resize((px, px), Image.LANCZOS)


COLOURS = dict(tile=TILE, back=BACK, front=FRONT, phone=PHONE, screen=SCREEN, dot=DOT)


def write_all(c=COLOURS):
    big = tile_icon(1024, c)
    # Windows
    res = ROOT / "pc" / "res"
    tile_icon(256, c).save(res / "icon.png", optimize=True)
    tile_icon(256, c).save(res / "icon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
    # Mac
    iconset = res / "mac" / "AppIcon.iconset"
    for n in (16, 32, 128, 256, 512):
        tile_icon(n, c).save(iconset / f"icon_{n}x{n}.png", optimize=True)
        tile_icon(n * 2, c).save(iconset / f"icon_{n}x{n}@2x.png", optimize=True)
    # Android
    icons = ROOT / "app" / "assets" / "icon"
    big.save(icons / "icon_1024.png", optimize=True)
    for name, f in {"mdpi": 1, "hdpi": 1.5, "xhdpi": 2, "xxhdpi": 3, "xxxhdpi": 4}.items():
        folder = icons / "android" / f"mipmap-{name}"
        folder.mkdir(parents=True, exist_ok=True)
        tile_icon(round(48 * f), c).save(folder / "ic_launcher.png", optimize=True)
        bg, fg = adaptive_layers(round(108 * f), c)
        bg.save(folder / "ic_launcher_background.png", optimize=True)
        fg.save(folder / "ic_launcher_foreground.png", optimize=True)
    anydpi = icons / "android" / "mipmap-anydpi-v26"
    anydpi.mkdir(parents=True, exist_ok=True)
    (anydpi / "ic_launcher.xml").write_text(
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">\n'
        '    <background android:drawable="@mipmap/ic_launcher_background"/>\n'
        '    <foreground android:drawable="@mipmap/ic_launcher_foreground"/>\n'
        '</adaptive-icon>\n')
    print("icons written")


if __name__ == "__main__":
    write_all()
