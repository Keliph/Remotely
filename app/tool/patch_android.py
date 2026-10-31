"""
Runs in the cloud build right after `flutter create` has generated the android/ folder.
Applies the Blanket Remote settings Flutter's template doesn't have:
  - internet + vibration permissions, plain-HTTP access to the PC on the local Wi-Fi
  - app name
  - our MainActivity (phone name + real vibration)
  - launcher icons
  - removes the template's sample test that refers to a class we don't have
"""
import pathlib
import re
import shutil
import sys

APP = pathlib.Path(__file__).resolve().parent.parent
ANDROID = APP / "android" / "app" / "src" / "main"


def patch_manifest():
    path = ANDROID / "AndroidManifest.xml"
    xml = path.read_text(encoding="utf-8")

    perms = [
        "android.permission.INTERNET",
        "android.permission.VIBRATE",
        "android.permission.ACCESS_NETWORK_STATE",
        "android.permission.ACCESS_WIFI_STATE",
    ]
    lines = "".join(f'\n    <uses-permission android:name="{p}"/>' for p in perms if p not in xml)
    xml, n = re.subn(r"(<manifest\b[^>]*>)", r"\1" + lines.replace("\\", "\\\\"), xml, count=1)
    if n != 1:
        sys.exit("patch_android: <manifest> tag not found")

    if "usesCleartextTraffic" not in xml:
        xml, n = re.subn(r"<application\b", '<application\n        android:usesCleartextTraffic="true"', xml, count=1)
        if n != 1:
            sys.exit("patch_android: <application> tag not found")

    xml = re.sub(r'android:label="[^"]*"', 'android:label="Blanket Remote"', xml, count=1)
    path.write_text(xml, encoding="utf-8")
    print("manifest patched")


def patch_activity():
    generated = list((ANDROID / "kotlin").rglob("MainActivity.kt"))
    if not generated:
        sys.exit("patch_android: generated MainActivity.kt not found")
    target = generated[0]
    package = re.search(r"^package\s+([\w.]+)", target.read_text(encoding="utf-8"), re.M)
    if not package:
        sys.exit("patch_android: package line not found in MainActivity.kt")
    template = (APP / "tool" / "MainActivity.kt.in").read_text(encoding="utf-8")
    target.write_text(template.replace("__PACKAGE__", package.group(1)), encoding="utf-8")
    print(f"MainActivity written ({package.group(1)})")


def cap_target_sdk(max_sdk=36):
    """Android 17 (SDK 37) blocks local-network access unless the app holds ACCESS_LOCAL_NETWORK.
    Until the app requests that permission, keep targeting SDK 36 so Wi-Fi discovery keeps working
    (apps targeting <= 36 keep local access through INTERNET).
    https://developer.android.com/privacy-and-security/local-network-permission"""
    app_dir = APP / "android" / "app"
    for name in ("build.gradle.kts", "build.gradle"):
        path = app_dir / name
        if not path.exists():
            continue
        text = path.read_text(encoding="utf-8")
        if name.endswith(".kts"):
            new, n = re.subn(r"targetSdk\s*=\s*flutter\.targetSdkVersion",
                             f"targetSdk = minOf(flutter.targetSdkVersion, {max_sdk})", text)
        else:
            new, n = re.subn(r"targetSdkVersion\s*(=\s*)?flutter\.targetSdkVersion",
                             f"targetSdkVersion Math.min(flutter.targetSdkVersion, {max_sdk})", text)
        if n:
            path.write_text(new, encoding="utf-8")
            print(f"targetSdk capped at {max_sdk} in {name}")
        elif "minOf(flutter.targetSdkVersion" in text or "Math.min(flutter.targetSdkVersion" in text:
            print("targetSdk already capped")
        else:
            print(f"WARNING: targetSdk line not found in {name}; check local-network access on Android 17")
        return
    print("WARNING: app build.gradle not found")


def patch_signing():
    """Sign release builds with the permanent key when BR_KEYSTORE points at it (set by the cloud build
    from the ANDROID_DEBUG_KEYSTORE secret). Without it, Flutter's default (a temporary debug key) is used."""
    app_dir = APP / "android" / "app"
    kts, groovy = app_dir / "build.gradle.kts", app_dir / "build.gradle"
    if kts.exists():
        path, text = kts, kts.read_text(encoding="utf-8")
        block = """
    signingConfigs {
        create("blanket") {
            val ks = System.getenv("BR_KEYSTORE")
            if (ks != null) {
                storeFile = file(ks)
                storePassword = "android"
                keyAlias = "androiddebugkey"
                keyPassword = "android"
            }
        }
    }
"""
        use = 'signingConfig = if (System.getenv("BR_KEYSTORE") != null) signingConfigs.getByName("blanket") else signingConfigs.getByName("debug")'
        old_use = re.compile(r'signingConfig\s*=\s*signingConfigs\.getByName\("debug"\)')
    elif groovy.exists():
        path, text = groovy, groovy.read_text(encoding="utf-8")
        block = """
    signingConfigs {
        blanket {
            if (System.getenv("BR_KEYSTORE") != null) {
                storeFile file(System.getenv("BR_KEYSTORE"))
                storePassword "android"
                keyAlias "androiddebugkey"
                keyPassword "android"
            }
        }
    }
"""
        use = 'signingConfig = System.getenv("BR_KEYSTORE") != null ? signingConfigs.blanket : signingConfigs.debug'
        old_use = re.compile(r'signingConfig\s*=?\s*signingConfigs\.debug\b')
    else:
        print("WARNING: app build.gradle not found; release signing not patched")
        return
    if "BR_KEYSTORE" in text:
        print("signing already patched")
        return
    text, n1 = re.subn(r"\n(\s*)buildTypes\s*\{", lambda m: block + "\n" + m.group(1) + "buildTypes {", text, count=1)
    text, n2 = old_use.subn(use, text, count=1)
    if n1 != 1 or n2 != 1:
        print(f"WARNING: release signing not patched in {path.name} (buildTypes={n1}, signingConfig={n2})")
        return
    path.write_text(text, encoding="utf-8")
    print(f"release signing uses BR_KEYSTORE when set ({path.name})")


def copy_icons():
    src = APP / "assets" / "icon" / "android"
    count = 0
    for icon in src.glob("mipmap-*/ic_launcher.png"):
        dest = ANDROID / "res" / icon.parent.name / "ic_launcher.png"
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(icon, dest)
        count += 1
    print(f"{count} launcher icons copied")


def drop_template_test():
    t = APP / "test" / "widget_test.dart"
    if t.exists() and "MyApp" in t.read_text(encoding="utf-8"):
        t.unlink()
        print("removed template widget_test.dart")


if __name__ == "__main__":
    patch_manifest()
    patch_activity()
    cap_target_sdk()
    patch_signing()
    copy_icons()
    drop_template_test()
