import 'package:flutter/material.dart';

import 'native.dart';
import 'pc_link.dart';
import 'remote_screen.dart';
import 'scan_view.dart';

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
    _quickStart();
    _scan();
  }

  /// Opens the remote right away if the PC used last time answers at its saved address with the
  /// saved key (one quick request). The full search below still runs for the list.
  Future<void> _quickStart() async {
    final lastId = await PcStore.lastUsed();
    if (lastId == null) return;
    final pc = await PcStore.byId(lastId);
    if (pc == null || !pc.paired) return;
    if (await checkPc(pc) != AuthCheck.ok || _autoOpened || !mounted) return;
    _autoOpened = true;
    pc.online = true;
    _openRemote(pc);
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
    // keep any key the remote screen saved meanwhile (e.g. after connecting again)
    for (final fresh in await PcStore.load()) {
      final p = byId[fresh.id];
      if (p != null && fresh.paired) p.key = fresh.key;
    }
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
    if (mounted) _scan(); // the PC's address or key may have changed while the remote was open
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
    return Scaffold(
      body: ScanView(
        pcs: _pcs,
        searching: _searching,
        busyId: _busyId,
        onTap: _tap,
        onForget: _forget,
        onSearch: _scan,
        onAdd: _addByIp,
      ),
    );
  }
}

extension _FirstOrNull<T> on List<T> {
  T? get firstOrNullCompat => isEmpty ? null : first;
}
