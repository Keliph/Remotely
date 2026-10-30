// Finding, pairing with and remembering PCs that run Blanket Remote.
// Protocol (see pc/README.md):
//   UDP broadcast "BLANKETREMOTE?" to port 47820 -> PC answers "BLANKETREMOTE!{json}"
//   POST /api/pair {"name": ...} -> PC shows Yes/No popup -> {"key": ...}
//   GET  /api/ping -> {"app":"BlanketRemote","id":...}
import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:shared_preferences/shared_preferences.dart';

const int discoveryPort = 47820;
const int defaultPcPort = 47801;
const String _ask = 'BLANKETREMOTE?';
const String _reply = 'BLANKETREMOTE!';

class Pc {
  Pc({required this.id, required this.name, required this.ip, required this.port, this.key, this.os = ''});

  final String id; // stable id of the PC, survives IP changes
  String name;
  String ip;
  int port;
  String? key; // given by the PC after the user clicks Yes
  String os; // 'windows' or 'darwin' (Mac)
  bool online = false;

  bool get paired => key != null && key!.isNotEmpty;
  bool get isMac => os == 'darwin';
  String get remoteUrl => 'http://$ip:$port/?key=${Uri.encodeComponent(key ?? '')}';

  Map<String, dynamic> toJson() => {'id': id, 'name': name, 'ip': ip, 'port': port, 'key': key, 'os': os};

  static Pc? fromJson(Object? raw) {
    if (raw is! Map) return null;
    final id = raw['id'], ip = raw['ip'];
    if (id is! String || ip is! String) return null;
    final port = raw['port'];
    final name = raw['name'];
    final key = raw['key'];
    final os = raw['os'];
    return Pc(
      id: id,
      ip: ip,
      name: name is String ? name : 'PC',
      port: port is int ? port : defaultPcPort,
      key: key is String ? key : null,
      os: os is String ? os : '',
    );
  }
}

// ---------- saved PCs ----------

class PcStore {
  static const _pcsKey = 'pcs';
  static const _lastKey = 'last_pc';

  static Future<List<Pc>> load() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_pcsKey);
    if (raw == null) return [];
    try {
      final list = jsonDecode(raw);
      if (list is! List) return [];
      return [for (final e in list) Pc.fromJson(e)].whereType<Pc>().toList();
    } catch (_) {
      return [];
    }
  }

  /// Only paired PCs are worth remembering.
  static Future<void> save(Iterable<Pc> pcs) async {
    final prefs = await SharedPreferences.getInstance();
    final data = [for (final p in pcs) if (p.paired) p.toJson()];
    await prefs.setString(_pcsKey, jsonEncode(data));
  }

  static Future<String?> lastUsed() async => (await SharedPreferences.getInstance()).getString(_lastKey);

  static Future<void> setLastUsed(String id) async =>
      (await SharedPreferences.getInstance()).setString(_lastKey, id);
}

// ---------- discovery ----------

/// Shouts "anyone there?" on the Wi-Fi and collects every PC that answers.
Future<List<Pc>> discoverPcs({Duration listen = const Duration(milliseconds: 1500)}) async {
  final found = <String, Pc>{};
  RawDatagramSocket? sock;
  try {
    final s = await RawDatagramSocket.bind(InternetAddress.anyIPv4, 0);
    sock = s;
    s.broadcastEnabled = true;
    s.listen((event) {
      if (event != RawSocketEvent.read) return;
      final dg = s.receive();
      if (dg == null) return;
      final text = utf8.decode(dg.data, allowMalformed: true);
      if (!text.startsWith(_reply)) return;
      try {
        final j = jsonDecode(text.substring(_reply.length));
        if (j is! Map) return;
        final id = j['id'];
        if (id is! String) return;
        final name = j['name'];
        final port = j['port'];
        final os = j['os'];
        found[id] = Pc(
          id: id,
          name: name is String ? name : 'PC',
          ip: dg.address.address,
          port: port is int ? port : defaultPcPort,
          os: os is String ? os : '',
        )..online = true;
      } catch (_) {}
    });

    final msg = utf8.encode(_ask);
    final nets = await _localNets();
    final broadcasts = <String>{'255.255.255.255', for (final p in nets.prefixes) '$p.255'};
    for (var round = 0; round < 3; round++) {
      for (final t in broadcasts) {
        _send(s, msg, t);
      }
      if (round == 0) {
        // Many routers don't pass broadcasts from Wi-Fi to the Ethernet ports, so also ask every
        // address on this network directly (254 tiny packets)...
        await _sweep(s, msg, nets.prefixes, nets.own);
        // ...and the usual home-router ranges, for a PC plugged into the ISP modem while the phone
        // uses a second router's Wi-Fi (a different network that broadcasts can't reach).
        await _sweep(s, msg, [for (final p in _commonPrefixes) if (!nets.prefixes.contains(p)) p], nets.own);
      }
      await Future<void>.delayed(const Duration(milliseconds: 250));
    }
    await Future<void>.delayed(listen);
  } catch (_) {
    // no Wi-Fi / no network: return whatever we have
  } finally {
    sock?.close();
  }
  return found.values.toList();
}

const List<String> _commonPrefixes = [
  '192.168.0', '192.168.1', '192.168.2', '192.168.8', '192.168.10', '192.168.18',
  '192.168.100', '192.168.137', '10.0.0', '10.0.1',
];

void _send(RawDatagramSocket s, List<int> msg, String ip) {
  try {
    s.send(msg, InternetAddress(ip), discoveryPort);
  } catch (_) {}
}

/// Sends the question to every address x.y.z.1-254 of each prefix, paced so the phone doesn't drop packets.
Future<void> _sweep(RawDatagramSocket s, List<int> msg, Iterable<String> prefixes, Set<String> own) async {
  var n = 0;
  for (final p in prefixes) {
    for (var i = 1; i < 255; i++) {
      final ip = '$p.$i';
      if (own.contains(ip)) continue;
      _send(s, msg, ip);
      n++;
      if (n % 48 == 0) await Future<void>.delayed(const Duration(milliseconds: 4));
    }
  }
}

class _Nets {
  _Nets(this.prefixes, this.own);
  final List<String> prefixes; // e.g. "192.168.1"
  final Set<String> own; // this phone's addresses
}

/// The phone's own local networks (Wi-Fi, hotspot, Ethernet). Mobile-data addresses are skipped.
Future<_Nets> _localNets() async {
  final prefixes = <String>{};
  final own = <String>{};
  try {
    final ifaces = await NetworkInterface.list(type: InternetAddressType.IPv4);
    final lanLike = RegExp(r'^(wlan|swlan|ap|eth|en|wl)');
    final lan = ifaces.where((i) => lanLike.hasMatch(i.name)).toList();
    for (final ifc in (lan.isNotEmpty ? lan : ifaces)) {
      for (final a in ifc.addresses) {
        if (a.isLoopback || !_isPrivate(a.address)) continue;
        final p = a.address.split('.');
        prefixes.add('${p[0]}.${p[1]}.${p[2]}');
        own.add(a.address);
      }
    }
  } catch (_) {}
  return _Nets(prefixes.toList(), own);
}

bool _isPrivate(String ip) {
  final p = ip.split('.').map(int.tryParse).toList();
  if (p.length != 4 || p.any((x) => x == null)) return false;
  final a = p[0]!, b = p[1]!;
  return a == 10 || (a == 172 && b >= 16 && b <= 31) || (a == 192 && b == 168);
}

// ---------- HTTP helpers ----------

/// Checks that a Blanket Remote PC answers at this address. Returns its info or null.
Future<Map<String, dynamic>?> pingPc(String ip, int port) async {
  final client = HttpClient()..connectionTimeout = const Duration(seconds: 2);
  try {
    final req = await client.getUrl(Uri.parse('http://$ip:$port/api/ping'));
    final res = await req.close().timeout(const Duration(seconds: 3));
    final body = await res.transform(utf8.decoder).join();
    final j = jsonDecode(body);
    if (j is Map<String, dynamic> && j['app'] == 'BlanketRemote') return j;
  } catch (_) {
  } finally {
    client.close(force: true);
  }
  return null;
}

enum PairResult { ok, denied, busy, unreachable }

class PairOutcome {
  PairOutcome(this.result, {this.key, this.message});
  final PairResult result;
  final String? key;
  final String? message;
}

/// Asks the PC for access. The PC shows a Yes/No popup; this waits for the answer.
Future<PairOutcome> pairWith(Pc pc, String phoneName) async {
  final client = HttpClient()..connectionTimeout = const Duration(seconds: 4);
  try {
    final req = await client.postUrl(Uri.parse('http://${pc.ip}:${pc.port}/api/pair'));
    req.headers.contentType = ContentType.json;
    req.write(jsonEncode({'name': phoneName}));
    final res = await req.close().timeout(const Duration(minutes: 2));
    final body = await res.transform(utf8.decoder).join();
    switch (res.statusCode) {
      case 200:
        final j = jsonDecode(body);
        final key = j is Map ? j['key'] : null;
        if (key is String && key.isNotEmpty) return PairOutcome(PairResult.ok, key: key);
        return PairOutcome(PairResult.unreachable);
      case 403:
        return PairOutcome(PairResult.denied);
      case 429:
        return PairOutcome(PairResult.busy, message: body.trim());
      default:
        return PairOutcome(PairResult.unreachable);
    }
  } catch (_) {
    return PairOutcome(PairResult.unreachable);
  } finally {
    client.close(force: true);
  }
}
