import 'package:blanket_remote/pc_link.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('saved PC survives a JSON round trip', () {
    final pc = Pc(id: 'abc123', name: 'DESKTOP-1', ip: '192.168.1.5', port: 47801, key: 'k');
    final back = Pc.fromJson(pc.toJson())!;
    expect(back.id, 'abc123');
    expect(back.paired, isTrue);
    expect(back.remoteUrl, 'http://192.168.1.5:47801/?key=k');
  });

  test('bad saved data is ignored', () {
    expect(Pc.fromJson({'name': 'x'}), isNull);
    expect(Pc.fromJson('nope'), isNull);
  });
}
