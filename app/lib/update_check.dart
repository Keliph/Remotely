// "New version" bar: every release publishes version.json next to the downloads. The app compares its own
// version (the time of the last change to the app, stamped in by the build) and offers the new APK.
import 'dart:convert';
import 'dart:io';

/// Set by the build: --dart-define=APP_VERSION=<unix time of the last change to app/>. 0 in dev builds.
const appVersion = int.fromEnvironment('APP_VERSION');

const _versionUrl = 'https://github.com/Keliph/Remotely/releases/download/latest/version.json';
const _downloadPrefix = 'https://github.com/Keliph/Remotely/releases/';

bool _askedThisRun = false;

/// The download link of a newer APK, or null. Asks at most once per app start.
Future<String?> newerApk() async {
  if (appVersion == 0 || _askedThisRun) return null;
  _askedThisRun = true;
  final client = HttpClient()..connectionTimeout = const Duration(seconds: 10);
  try {
    final req = await client.getUrl(Uri.parse(_versionUrl));
    final resp = await req.close().timeout(const Duration(seconds: 15));
    if (resp.statusCode != 200) return null;
    final body = await resp.transform(utf8.decoder).join().timeout(const Duration(seconds: 15));
    final info = jsonDecode(body);
    if (info is! Map) return null;
    final latest = info['app'], apk = info['apk'];
    if (latest is int && latest > appVersion && apk is String && apk.startsWith(_downloadPrefix)) return apk;
  } catch (_) {
    // offline or GitHub unreachable: just don't show the bar
  } finally {
    client.close(force: true);
  }
  return null;
}
