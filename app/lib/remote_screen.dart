import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:webview_flutter/webview_flutter.dart';

import 'native.dart';
import 'pc_link.dart';
import 'ui.dart';

/// Shows the remote page served by the PC itself, so the PC app and the
/// phone app always speak the same version. The page talks back through
/// the "BRApp" channel for vibration and the shutdown dialog.
class RemoteScreen extends StatefulWidget {
  const RemoteScreen({super.key, required this.pc});
  final Pc pc;

  @override
  State<RemoteScreen> createState() => _RemoteScreenState();
}

class _RemoteScreenState extends State<RemoteScreen> {
  late final WebViewController _web;
  bool _failed = false;

  @override
  void initState() {
    super.initState();
    _web = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setBackgroundColor(const Color(0xFF2A1C16))
      ..addJavaScriptChannel('BRApp', onMessageReceived: _onPageMessage)
      ..setNavigationDelegate(NavigationDelegate(
        onWebResourceError: (WebResourceError error) {
          if (error.isForMainFrame ?? true) {
            if (mounted) setState(() => _failed = true);
          }
        },
      ))
      ..loadRequest(Uri.parse(widget.pc.remoteUrl));
  }

  void _onPageMessage(JavaScriptMessage message) {
    Object? data;
    try {
      data = jsonDecode(message.message);
    } catch (_) {
      return;
    }
    if (data is! Map) return;
    final h = data['h'];
    if (h is String) {
      Native.haptic(h);
      return;
    }
    if (data['confirm'] == 'shutdown') {
      final text = data['text'];
      _confirmShutdown(text is String ? text : 'Shut down this PC?');
    }
  }

  Future<void> _confirmShutdown(String text) async {
    final yes = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        content: Text(text, style: const TextStyle(fontSize: 17)),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c, false), child: const Text('Cancel')),
          TextButton(onPressed: () => Navigator.pop(c, true), child: const Text('Shut down')),
        ],
      ),
    );
    if (yes == true) {
      await _web.runJavaScript("window.brConfirmed && window.brConfirmed('shutdown')");
    }
  }

  void _retry() {
    setState(() => _failed = false);
    _web.loadRequest(Uri.parse(widget.pc.remoteUrl));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF2A1C16),
      body: SafeArea(
        child: Stack(children: [
          Positioned.fill(child: WebViewWidget(controller: _web)),
          if (_failed) Positioned.fill(child: _errorView()),
        ]),
      ),
    );
  }

  Widget _errorView() {
    return WarmBackground(
      child: Center(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Glass(
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text("Can't reach ${widget.pc.name}", style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w300)),
              const SizedBox(height: 10),
              const Text('Check the PC is on, Blanket Remote is running, and this phone is on the same Wi-Fi.',
                  style: TextStyle(fontSize: 15, color: ink2, height: 1.4)),
              const SizedBox(height: 20),
              Row(children: [
                Expanded(child: PillButton(label: 'Try again', solid: true, onPressed: _retry)),
                const SizedBox(width: 10),
                Expanded(child: PillButton(label: 'Back', onPressed: () => Navigator.of(context).pop())),
              ]),
            ]),
          ),
        ),
      ),
    );
  }
}
