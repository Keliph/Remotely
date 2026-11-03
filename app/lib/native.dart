// Small bridge to Android code in MainActivity.kt (phone name + real vibration).
// On platforms without it (e.g. iPhone for now) it falls back to Flutter's haptics.
import 'package:flutter/services.dart';

class Native {
  static const _channel = MethodChannel('blanketremote/native');

  /// e.g. "Samsung Galaxy A15" — shown in the PC's "Allow ...?" popup.
  static Future<String> phoneName() async {
    try {
      final name = await _channel.invokeMethod<String>('model');
      if (name != null && name.trim().isNotEmpty) return name.trim();
    } catch (_) {}
    return 'Phone';
  }

  /// Opens a link in the app that handles it (WhatsApp for wa.me links, otherwise the browser).
  static Future<void> openUrl(String url) async {
    try {
      await _channel.invokeMethod<void>('openUrl', {'url': url});
    } catch (_) {}
  }

  /// kind: press | tap | rightTap | drag | notch
  static Future<void> haptic(String kind) async {
    try {
      await _channel.invokeMethod<void>('haptic', {'kind': kind});
    } catch (_) {
      if (kind == 'drag' || kind == 'notch') {
        await HapticFeedback.selectionClick();
      } else {
        await HapticFeedback.lightImpact();
      }
    }
  }
}
