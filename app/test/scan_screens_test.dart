// Renders the scan screen in its main states to PNG files (build/screens/), so the cloud build can
// show what it looks like. Also fails if anything overflows on a small or a tall phone.
import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:blanket_remote/pc_link.dart';
import 'package:blanket_remote/scan_view.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

Future<void> _loadFonts() async {
  final outfit = FontLoader('Outfit');
  for (final w in [300, 400, 500]) {
    outfit.addFont(rootBundle.load('assets/fonts/Outfit-$w.ttf'));
  }
  await outfit.load();
  // Material icons ship with the Flutter SDK
  var dir = File(Platform.resolvedExecutable).parent;
  for (var i = 0; i < 10; i++) {
    final f = File('${dir.path}/bin/cache/artifacts/material_fonts/MaterialIcons-Regular.otf');
    if (f.existsSync()) {
      final icons = FontLoader('MaterialIcons')..addFont(Future.value(ByteData.sublistView(f.readAsBytesSync())));
      await icons.load();
      return;
    }
    dir = dir.parent;
  }
}

Pc _pc(String id, String name, {bool online = true, bool paired = true, String os = 'windows'}) =>
    Pc(id: id, name: name, ip: '192.168.1.5', port: 47801, key: paired ? 'k' : null, os: os)..online = online;

void main() {
  final states = <String, (List<Pc>, bool, String?)>{
    'searching': (<Pc>[], true, null),
    'found': (
      [
        _pc('a', 'ASUS-VIVOBOOK'),
        _pc('b', "Epuh's MacBook Pro", paired: false, os: 'darwin'),
        _pc('c', 'OFFICE-PC', online: false),
      ],
      false,
      null
    ),
    'pairing': ([_pc('b', "Epuh's MacBook Pro", paired: false, os: 'darwin')], false, 'b'),
    'empty': (<Pc>[], false, null),
  };
  final phones = {'tall': const Size(412, 915), 'small': const Size(360, 640)};

  for (final phone in phones.entries) {
    for (final state in states.entries) {
      testWidgets('scan screen: ${state.key} on ${phone.key} phone', (tester) async {
        await _loadFonts();
        tester.view.physicalSize = phone.value * 2;
        tester.view.devicePixelRatio = 2;
        tester.view.padding = const FakeViewPadding(top: 48, bottom: 32);
        addTearDown(tester.view.reset);
        final (pcs, searching, busy) = state.value;
        final key = GlobalKey();
        await tester.pumpWidget(RepaintBoundary(
          key: key,
          child: MaterialApp(
            debugShowCheckedModeBanner: false,
            theme: ThemeData(brightness: Brightness.dark, fontFamily: 'Outfit'),
            home: Scaffold(
              body: ScanView(
                pcs: pcs,
                searching: searching,
                busyId: busy,
                onTap: (_) {},
                onForget: (_) {},
                onSearch: () async {},
                onAdd: () {},
              ),
            ),
          ),
        ));
        await tester.pump(const Duration(milliseconds: 1400));
        expect(tester.takeException(), isNull);
        await tester.runAsync(() async {
          final boundary = key.currentContext!.findRenderObject()! as RenderRepaintBoundary;
          final image = await boundary.toImage(pixelRatio: 2);
          final png = await image.toByteData(format: ui.ImageByteFormat.png);
          File('build/screens/${phone.key}-${state.key}.png')
            ..createSync(recursive: true)
            ..writeAsBytesSync(png!.buffer.asUint8List());
        });
      });
    }
  }
}
