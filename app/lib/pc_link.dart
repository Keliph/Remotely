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
  Pc({required this.id, required this.name, required this.ip, required this.port, this.key});

  final String id; // stable id of the PC, survives IP changes
  String name;
  String ip;
  int port;
  String? key; // given by the PC after the user clicks Yes
  bool online = false;

  bool get paired => key != null && key!.isNotEmpty;
  String get remoteUrl => 'http://$ip:$port/?key=${Uri.encodeComponent(key ?? '')}';

  Map<String, dynamic> toJson() => {'id': id, 'name': name, 'ip': ip, 'port': port, 'key': key};

  static Pc? fromJson(Object? raw) {
    if (raw is! Map) return null;
    final id = raw['id'], ip = raw['ip'];
    if (id is! String || ip is! String) return null;
    final port = raw['port'];
    final name = raw['name'];
    final key = raw['key'];
    return Pc(
      id: id,
      ip: ip,
      name: name is String ? name : 'PC',
      port: port is int ? port : defaultPcPort,
      key: key is String ? key : null,
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
        found[id] = Pc(
          id: id,
          name: name is String ? name : 'PC',
          ip: dg.address.address,
          port: port is int ? port : defaultPcPort,
        )..online = true;
      } catch (_) {}
    });

    final msg = utf8.encode(_ask);
    final targets = <String>{'255.255.255.255', ...await _subnetBroadcasts()};
    for (var round = 0; round < 3; round++) {
      for (final t in targets) {
        try {
          s.send(msg, InternetAddress(t), discoveryPort);
        } catch (_) {}
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

/// Some phones won't send to 255.255.255.255, so also try e.g. 192.168.1.255.
Future<List<String>> _subnetBroadcasts() async {
  final out = <String>[];
  try {
    final ifaces = await NetworkInterface.list(type: InternetAddressType.IPv4);
    for (final ifc in ifaces) {
      for (final a in ifc.addresses) {
        final p = a.address.split('.');
        if (p.length == 4 && !a.isLoopback) out.add('${p[0]}.${p[1]}.${p[2]}.255');
      }
    }
  } catch (_) {}
  return out;
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
