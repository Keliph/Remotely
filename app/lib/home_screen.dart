import 'package:flutter/material.dart';

import 'native.dart';
import 'pc_link.dart';
import 'remote_screen.dart';
import 'ui.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  List<Pc> _pcs = [];
  bool _searching = false;
  bool _autoOpened = false;
  String? _busyId; // PC we're pairing with

  @override
  void initState() {
    super.initState();
    _scan();
  }

  // ---------- finding PCs ----------

  Future<void> _scan() async {
    if (_searching) return;
    setState(() => _searching = true);

    final saved = await PcStore.load();
    final found = await discoverPcs();
    final byId = <String, Pc>{for (final p in saved) p.id: p};
    for (final f in found) {
      final s = byId[f.id];
      if (s != null) {
        s
          ..ip = f.ip
          ..port = f.port
          ..name = f.name
          ..os = f.os
          ..online = true;
      } else {
        byId[f.id] = f;
      }
    }
    // Saved PCs that didn't answer the broadcast: try their last known address directly.
    await Future.wait([
      for (final p in byId.values)
        if (!p.online)
          pingPc(p.ip, p.port).then((info) {
            if (info != null && info['id'] == p.id) {
              p.online = true;
              if (info['os'] is String) p.os = info['os'] as String;
            }
          }),
    ]);
    await PcStore.save(byId.values);

    final list = byId.values.toList()
      ..sort((a, b) {
        if (a.online != b.online) return a.online ? -1 : 1;
        return a.name.toLowerCase().compareTo(b.name.toLowerCase());
      });
    if (!mounted) return;
    setState(() {
      _pcs = list;
      _searching = false;
    });

    // "Works in a second": jump straight into the PC you used last time.
    if (!_autoOpened) {
      _autoOpened = true;
      final lastId = await PcStore.lastUsed();
      for (final p in list) {
        if (p.id == lastId && p.online && p.paired) {
          _openRemote(p);
          break;
        }
      }
    }
  }

  // ---------- actions ----------

  Future<void> _tap(Pc pc) async {
    if (!pc.online) {
      _say("${pc.name} isn't answering. Is Blanket Remote running on it, on this Wi-Fi?");
      _scan();
      return;
    }
    if (pc.paired) {
      _openRemote(pc);
    } else {
      await _pair(pc);
    }
  }

  Future<void> _pair(Pc pc) async {
    setState(() => _busyId = pc.id);
    final phone = await Native.phoneName();
    final outcome = await pairWith(pc, phone);
    if (!mounted) return;
    setState(() => _busyId = null);
    switch (outcome.result) {
      case PairResult.ok:
        pc.key = outcome.key;
        pc.online = true;
        await PcStore.save(_pcs);
        if (mounted) _openRemote(pc);
      case PairResult.denied:
        _say('${pc.name} said no.');
      case PairResult.busy:
        _say((outcome.message?.isNotEmpty ?? false) ? outcome.message! : 'The PC is busy. Try again in a moment.');
      case PairResult.unreachable:
        _say("Couldn't reach ${pc.name}. Check both are on the same Wi-Fi.");
    }
  }

  Future<void> _openRemote(Pc pc) async {
    await PcStore.setLastUsed(pc.id);
    if (!mounted) return;
    await Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => RemoteScreen(pc: pc)));
  }

  Future<void> _forget(Pc pc) async {
    final yes = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        title: Text('Forget ${pc.name}?'),
        content: const Text("You'll need to click Yes on the PC again next time."),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c, false), child: const Text('Cancel')),
          TextButton(onPressed: () => Navigator.pop(c, true), child: const Text('Forget')),
        ],
      ),
    );
    if (yes != true) return;
    pc.key = null;
    await PcStore.save(_pcs);
    if (!mounted) return;
    setState(() {
      if (!pc.online) _pcs.remove(pc);
    });
  }

  Future<void> _addByIp() async {
    final field = TextEditingController();
    final text = await showDialog<String>(
      context: context,
      builder: (c) => AlertDialog(
        title: const Text('Add PC by address'),
        content: TextField(
          controller: field,
          autofocus: true,
          keyboardType: TextInputType.url,
          decoration: const InputDecoration(hintText: '192.168.1.5'),
          onSubmitted: (v) => Navigator.pop(c, v),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c), child: const Text('Cancel')),
          TextButton(onPressed: () => Navigator.pop(c, field.text), child: const Text('Add')),
        ],
      ),
    );
    final input = text?.trim() ?? '';
    if (input.isEmpty) return;

    // accept "192.168.1.5", "192.168.1.5:47801" or a pasted phone link
    var host = input.replaceFirst(RegExp(r'^https?://'), '').split('/').first;
    var port = defaultPcPort;
    final colon = host.lastIndexOf(':');
    if (colon > 0) {
      port = int.tryParse(host.substring(colon + 1)) ?? defaultPcPort;
      host = host.substring(0, colon);
    }
    final info = await pingPc(host, port);
    if (!mounted) return;
    if (info == null) {
      _say('No Blanket Remote at $host. Check the address on the PC page.');
      return;
    }
    final id = info['id'] is String ? info['id'] as String : host;
    final name = info['name'] is String ? info['name'] as String : 'PC';
    var pc = _pcs.where((p) => p.id == id).toList().firstOrNullCompat;
    if (pc == null) {
      pc = Pc(id: id, name: name, ip: host, port: port, os: info['os'] is String ? info['os'] as String : '');
      _pcs.insert(0, pc);
    }
    pc
      ..ip = host
      ..port = port
      ..online = true;
    // a pasted phone link from the PC page already carries the key: no need to click Yes
    final linkKey = RegExp(r'[?&]key=([^&#\s]+)').firstMatch(input)?.group(1);
    if (linkKey != null && linkKey.isNotEmpty) {
      pc.key = Uri.decodeComponent(linkKey);
      await PcStore.save(_pcs);
    }
    setState(() {});
    await _tap(pc);
  }

  void _say(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(msg)));
  }

  // ---------- UI ----------

  @override
  Widget build(BuildContext context) {
    final status = _searching
        ? 'Looking for your PC…'
        : _pcs.isEmpty
            ? 'No PC found on this Wi-Fi'
            : 'Tap a PC to control it';

    return Scaffold(
      body: WarmBackground(
        child: SafeArea(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(28, 28, 28, 6),
                child: Text('Blanket Remote',
                    style: const TextStyle(fontSize: 32, fontWeight: FontWeight.w300, color: ink, height: 1.1)),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(28, 0, 28, 18),
                child: Row(children: [
                  if (_searching) ...[
                    const SizedBox(
                        width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 1.6, color: ink2)),
                    const SizedBox(width: 10),
                  ],
                  Text(status, style: const TextStyle(fontSize: 15, color: ink2)),
                ]),
              ),
              Expanded(
                child: RefreshIndicator(
                  onRefresh: _scan,
                  color: const Color(0xFF1D1410),
                  backgroundColor: Colors.white,
                  child: ListView(
                    physics: const AlwaysScrollableScrollPhysics(),
                    padding: const EdgeInsets.fromLTRB(14, 0, 14, 20),
                    children: [
                      for (final pc in _pcs) ...[_pcCard(pc), const SizedBox(height: 10)],
                      if (!_searching && _pcs.isEmpty) _emptyCard(),
                    ],
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(14, 0, 14, 14),
                child: Row(children: [
                  Expanded(
                    child: PillButton(
                        label: 'Search again', icon: Icons.refresh_rounded, onPressed: _searching ? null : _scan),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: PillButton(label: 'Add by address', icon: Icons.add_rounded, onPressed: _addByIp),
                  ),
                ]),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _pcCard(Pc pc) {
    final busy = _busyId == pc.id;
    final String sub;
    if (busy) {
      sub = 'Click Yes on your PC screen';
    } else if (!pc.online) {
      sub = 'Not on this Wi-Fi right now';
    } else if (pc.paired) {
      sub = 'Ready';
    } else {
      sub = 'Tap to connect';
    }
    return Glass(
      onTap: busy ? null : () => _tap(pc),
      onLongPress: pc.paired ? () => _forget(pc) : null,
      child: Row(children: [
        Container(
          width: 48,
          height: 48,
          decoration: const BoxDecoration(color: pill, shape: BoxShape.circle),
          child: Icon(pc.isMac ? Icons.laptop_mac_rounded : Icons.laptop_windows_rounded,
              color: pc.online ? ink : ink3, size: 24),
        ),
        const SizedBox(width: 16),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(pc.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(fontSize: 19, fontWeight: FontWeight.w400, color: pc.online ? ink : ink3)),
            const SizedBox(height: 2),
            Text(sub, style: const TextStyle(fontSize: 14, color: ink2)),
          ]),
        ),
        if (busy)
          const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2, color: ink))
        else
          Icon(Icons.chevron_right_rounded, color: pc.online ? ink2 : ink3),
      ]),
    );
  }

  Widget _emptyCard() {
    return const Glass(
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('Get your PC ready', style: TextStyle(fontSize: 19, color: ink)),
        SizedBox(height: 10),
        Text('1. Open Blanket Remote on your PC or Mac.', style: TextStyle(fontSize: 15, color: ink2, height: 1.5)),
        Text('2. Keep this phone on the same Wi-Fi.', style: TextStyle(fontSize: 15, color: ink2, height: 1.5)),
        Text('3. Pull down here to search again.', style: TextStyle(fontSize: 15, color: ink2, height: 1.5)),
      ]),
    );
  }
}

extension _FirstOrNull<T> on List<T> {
  T? get firstOrNullCompat => isEmpty ? null : first;
}
