import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:webview_flutter/webview_flutter.dart';

import 'native.dart';
import 'pc_link.dart';
import 'ui.dart';

/// Shows the remote page served by the PC itself, so the PC app and the phone app always speak the
/// same version. The page talks back through the "BRApp" channel (vibration, shutdown dialog, and
/// "I lost the PC"). When the PC disappears — restarted, woke up with a new IP, Wi-Fi blip — this
/// screen finds it again by its id and reconnects by itself.
class RemoteScreen extends StatefulWidget {
  const RemoteScreen({super.key, required this.pc});
  final Pc pc;

  @override
  State<RemoteScreen> createState() => _RemoteScreenState();
}

enum _Phase { live, searching, needsPair }

class _RemoteScreenState extends State<RemoteScreen> {
  late final WebViewController _web;
  _Phase _phase = _Phase.live;
  bool _recovering = false;
  bool _pairing = false;
  Timer? _retry;

  Pc get _pc => widget.pc;

  @override
  void initState() {
    super.initState();
    _web = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setBackgroundColor(const Color(0xFF2A1C16))
      ..addJavaScriptChannel('BRApp', onMessageReceived: _onPageMessage)
      ..setNavigationDelegate(NavigationDelegate(
        onWebResourceError: (WebResourceError error) {
          if (error.isForMainFrame ?? true) _recover();
        },
      ));
    _load();
  }

  @override
  void dispose() {
    _retry?.cancel();
    super.dispose();
  }

  void _load() => _web.loadRequest(Uri.parse(_pc.remoteUrl));

  void _setPhase(_Phase p) {
    if (mounted && _phase != p) setState(() => _phase = p);
  }

  // ---------- finding the PC again ----------

  Future<void> _recover() async {
    if (_recovering || !mounted) return;
    _recovering = true;
    _retry?.cancel();
    try {
      var result = await checkPc(_pc);
      if (result == AuthCheck.notThere && await relocatePc(_pc)) {
        await PcStore.upsert(_pc); // remember the new address
        result = await checkPc(_pc);
      }
      if (!mounted) return;
      switch (result) {
        case AuthCheck.ok:
          _setPhase(_Phase.live);
          _load();
        case AuthCheck.wrongKey:
          _setPhase(_Phase.needsPair);
        case AuthCheck.notThere:
          _setPhase(_Phase.searching);
          _retry = Timer(const Duration(seconds: 4), _recover); // keep looking until the PC is back
      }
    } finally {
      _recovering = false;
    }
  }

  // The PC made a new key (its "New key" button), so the saved one stopped working.
  Future<void> _pairAgain() async {
    setState(() => _pairing = true);
    final outcome = await pairWith(_pc, await Native.phoneName());
    if (!mounted) return;
    setState(() => _pairing = false);
    if (outcome.result == PairResult.ok) {
      _pc.key = outcome.key;
      await PcStore.upsert(_pc);
      _setPhase(_Phase.live);
      _load();
      return;
    }
    final msg = switch (outcome.result) {
      PairResult.denied => '${_pc.name} said no.',
      PairResult.busy => 'The PC is busy. Try again in a moment.',
      _ => "Couldn't reach ${_pc.name}.",
    };
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(msg)));
  }

  // ---------- messages from the page ----------

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
    if (data['lost'] == true) {
      _recover();
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

  // ---------- UI ----------

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF2A1C16),
      body: SafeArea(
        child: Stack(children: [
          Positioned.fill(child: WebViewWidget(controller: _web)),
          if (_phase == _Phase.searching) Positioned.fill(child: _searchingView()),
          if (_phase == _Phase.needsPair) Positioned.fill(child: _needsPairView()),
        ]),
      ),
    );
  }

  Widget _panel(List<Widget> children) {
    return WarmBackground(
      child: Center(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Glass(
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: children),
          ),
        ),
      ),
    );
  }

  Widget _searchingView() {
    return _panel([
      Row(children: [
        const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 1.8, color: ink)),
        const SizedBox(width: 14),
        Expanded(
          child: Text('Looking for ${_pc.name}…',
              style: const TextStyle(fontSize: 21, fontWeight: FontWeight.w300), overflow: TextOverflow.ellipsis),
        ),
      ]),
      const SizedBox(height: 12),
      const Text(
          'It connects by itself as soon as the PC is on, Blanket Remote is running, and this phone is on the same Wi-Fi.',
          style: TextStyle(fontSize: 15, color: ink2, height: 1.4)),
      const SizedBox(height: 20),
      PillButton(label: 'Back', onPressed: () => Navigator.of(context).pop()),
    ]);
  }

  Widget _needsPairView() {
    return _panel([
      Text('${_pc.name} has a new key', style: const TextStyle(fontSize: 21, fontWeight: FontWeight.w300)),
      const SizedBox(height: 10),
      const Text('Connect again and click Allow on the PC. You only do this once.',
          style: TextStyle(fontSize: 15, color: ink2, height: 1.4)),
      const SizedBox(height: 20),
      Row(children: [
        Expanded(
          child: PillButton(
              label: _pairing ? 'Click Allow on the PC…' : 'Connect again',
              solid: true,
              onPressed: _pairing ? null : _pairAgain),
        ),
        const SizedBox(width: 10),
        Expanded(child: PillButton(label: 'Back', onPressed: () => Navigator.of(context).pop())),
      ]),
    ]);
  }
}
