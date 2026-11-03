// The first screen: finds PCs on the Wi-Fi. Drawn like the remote itself: the glowing amber card on
// top (its dark glow wanders while searching, like a finger on the touchpad) and a frosted dock with
// the PCs and buttons underneath.
import 'dart:math' as math;
import 'dart:ui' show ImageFilter;

import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart' show Ticker;

import 'pc_link.dart';
import 'ui.dart';

class ScanView extends StatelessWidget {
  const ScanView({
    super.key,
    required this.pcs,
    required this.searching,
    required this.busyId,
    required this.onTap,
    required this.onForget,
    required this.onSearch,
    required this.onAdd,
  });

  final List<Pc> pcs;
  final bool searching;
  final String? busyId; // PC we're pairing with
  final void Function(Pc) onTap;
  final void Function(Pc) onForget;
  final Future<void> Function() onSearch;
  final VoidCallback onAdd;

  @override
  Widget build(BuildContext context) {
    return WarmBackground(
      child: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(10),
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            Expanded(child: _hero()),
            const SizedBox(height: 10),
            _dock(context),
          ]),
        ),
      ),
    );
  }

  // ---------- the amber card ----------

  Widget _hero() {
    final online = pcs.where((p) => p.online).toList();
    final String sub, name;
    if (searching) {
      sub = 'Looking for';
      name = 'your PC…';
    } else if (online.length == 1) {
      sub = 'Found';
      name = online.first.name;
    } else if (online.length > 1) {
      sub = 'Found';
      name = '${online.length} PCs';
    } else if (pcs.isNotEmpty) {
      sub = 'No PC answering';
      name = 'Check your Wi-Fi';
    } else {
      sub = 'No PC found';
      name = 'on this Wi-Fi';
    }
    final hints = pcs.isEmpty
        ? const ['Open Blanket Remote on your PC or Mac', 'Keep this phone on the same Wi-Fi', 'Your PC shows up below']
        : [
            'Tap your PC to control it',
            if (pcs.any((p) => p.paired)) 'Hold it to forget it',
          ];

    const softShadow = [Shadow(color: Color(0x405A2800), blurRadius: 8, offset: Offset(0, 1))];
    const bigShadow = [Shadow(color: Color(0x4D5A2400), blurRadius: 12, offset: Offset(0, 1))];

    return DecoratedBox(
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(42),
        boxShadow: const [BoxShadow(color: Color(0x47000000), blurRadius: 48, offset: Offset(0, 24))],
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(42),
        child: Stack(fit: StackFit.expand, children: [
          const CustomPaint(painter: LayersPainter(padLayers)),
          _Glow(searching: searching),
          // thin light along the top edge, like the page's inset highlight
          const DecoratedBox(
            decoration: BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment(0, -0.97),
                colors: [Color(0x59FFFFFF), Color(0x00FFFFFF)],
              ),
            ),
          ),
          Positioned(
            left: 28,
            top: 26,
            right: 28,
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              const Text('Blanket Remote',
                  style: TextStyle(fontSize: 16, color: Color(0xEBFFFFFF), shadows: softShadow)),
              const SizedBox(height: 9),
              for (final h in hints)
                Padding(
                  padding: const EdgeInsets.only(bottom: 2),
                  child: Text(h, style: const TextStyle(fontSize: 14, color: ink2, height: 1.3, shadows: softShadow)),
                ),
            ]),
          ),
          Positioned(
            left: 28,
            right: 28,
            bottom: 26,
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Row(children: [
                if (searching) ...[
                  const SizedBox(width: 11, height: 11, child: CircularProgressIndicator(strokeWidth: 1.4, color: ink2)),
                  const SizedBox(width: 7),
                ],
                Text(sub, style: const TextStyle(fontSize: 13, color: ink2, letterSpacing: .13, shadows: bigShadow)),
              ]),
              const SizedBox(height: 1),
              Text(name,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                      fontSize: 21, fontWeight: FontWeight.w300, height: 1.15, color: ink, shadows: bigShadow)),
            ]),
          ),
        ]),
      ),
    );
  }

  // ---------- the frosted dock ----------

  Widget _dock(BuildContext context) {
    final maxList = math.min(250.0, MediaQuery.sizeOf(context).height * .38);
    return ClipRRect(
      borderRadius: BorderRadius.circular(34),
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 28, sigmaY: 28),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: const Color(0x21FFFFFF),
            borderRadius: BorderRadius.circular(34),
            border: Border.all(color: const Color(0x29FFFFFF)),
          ),
          child: Padding(
            padding: const EdgeInsets.all(11),
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              if (pcs.isNotEmpty) ...[
                ConstrainedBox(
                  constraints: BoxConstraints(maxHeight: maxList),
                  child: RefreshIndicator(
                    onRefresh: onSearch,
                    color: solidInk,
                    backgroundColor: Colors.white,
                    child: ListView.separated(
                      shrinkWrap: true,
                      padding: EdgeInsets.zero,
                      physics: const AlwaysScrollableScrollPhysics(),
                      itemCount: pcs.length,
                      separatorBuilder: (_, __) => const SizedBox(height: 8),
                      itemBuilder: (_, i) => _pcRow(pcs[i]),
                    ),
                  ),
                ),
                const SizedBox(height: 9),
              ],
              _buttons(),
            ]),
          ),
        ),
      ),
    );
  }

  Widget _pcRow(Pc pc) {
    final busy = busyId == pc.id;
    final ready = pc.online && pc.paired;
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
    return Material(
      color: pill,
      borderRadius: BorderRadius.circular(24),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: busy ? null : () => onTap(pc),
        onLongPress: pc.paired ? () => onForget(pc) : null,
        highlightColor: pillPress,
        splashColor: const Color(0x14FFFFFF),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(10, 10, 16, 10),
          child: Row(children: [
            Container(
              width: 46,
              height: 46,
              decoration: BoxDecoration(
                // the one white button, like the remote's send button: this PC is ready to go
                color: ready ? Colors.white : const Color(0x24FFFFFF),
                shape: BoxShape.circle,
              ),
              child: Icon(pc.isMac ? Icons.laptop_mac_rounded : Icons.laptop_windows_rounded,
                  size: 22, color: ready ? solidInk : (pc.online ? ink : ink3)),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(pc.name,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(fontSize: 17, color: pc.online ? ink : ink3)),
                const SizedBox(height: 1),
                Text(sub, style: const TextStyle(fontSize: 13, color: ink2)),
              ]),
            ),
            if (busy)
              const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 1.8, color: ink))
            else
              Icon(Icons.chevron_right_rounded, color: pc.online ? ink2 : ink3),
          ]),
        ),
      ),
    );
  }

  /// "Search again | Add by address", built like the remote's "Left click | Right click" bar.
  Widget _buttons() {
    Widget half(String label, IconData icon, VoidCallback? onPressed) => Expanded(
          child: InkWell(
            onTap: onPressed,
            highlightColor: pillPress,
            splashColor: const Color(0x14FFFFFF),
            child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
              Icon(icon, size: 19, color: onPressed == null ? ink3 : ink2),
              const SizedBox(width: 7),
              Text(label, style: TextStyle(fontSize: 15, color: onPressed == null ? ink3 : ink2)),
            ]),
          ),
        );
    return SizedBox(
      height: 46,
      child: Material(
        color: pill,
        shape: const StadiumBorder(),
        clipBehavior: Clip.antiAlias,
        child: Row(children: [
          half(searching ? 'Searching…' : 'Search again', Icons.refresh_rounded, searching ? null : () => onSearch()),
          Container(width: 1, color: const Color(0x1AFFFFFF)),
          half('Add by address', Icons.add_rounded, onAdd),
        ]),
      ),
    );
  }
}

/// The dark core of the glow (#core in remote.html). While searching it wanders around the card;
/// otherwise it eases back to its resting spot and stops.
class _Glow extends StatefulWidget {
  const _Glow({required this.searching});
  final bool searching;

  @override
  State<_Glow> createState() => _GlowState();
}

class _GlowState extends State<_Glow> with SingleTickerProviderStateMixin {
  static const rest = Offset(.34, .36); // same resting spot as the remote
  final _pos = ValueNotifier<Offset>(rest);
  late final Ticker _ticker = createTicker(_tick);
  Duration _last = Duration.zero;
  double _t = 0;

  bool get _wander => widget.searching && !(MediaQuery.maybeDisableAnimationsOf(context) ?? false);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _sync();
  }

  @override
  void didUpdateWidget(_Glow old) {
    super.didUpdateWidget(old);
    _sync();
  }

  void _sync() {
    if (!_ticker.isActive && (_wander || _pos.value != rest)) {
      _last = Duration.zero;
      _ticker.start();
    }
  }

  void _tick(Duration elapsed) {
    final dt = ((elapsed - _last).inMicroseconds / 1e6).clamp(0.0, 0.1).toDouble();
    _last = elapsed;
    _t += dt;
    final target = _wander
        ? Offset(.42 + .22 * math.sin(_t * 2 * math.pi / 4.6), .42 + .18 * math.sin(_t * 2 * math.pi / 3.3 + .8))
        : rest;
    final follow = 1 - math.pow(1 - (_wander ? .05 : .08), dt * 60).toDouble();
    final p = _pos.value + (target - _pos.value) * follow;
    if (!_wander && (target - p).distance < .0015) {
      _pos.value = rest;
      _ticker.stop();
    } else {
      _pos.value = p;
    }
  }

  @override
  void dispose() {
    _ticker.dispose();
    _pos.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return RepaintBoundary(
      child: ImageFiltered(
        imageFilter: ImageFilter.blur(sigmaX: 16, sigmaY: 16, tileMode: TileMode.decal),
        child: CustomPaint(painter: _CorePainter(_pos)),
      ),
    );
  }
}

class _CorePainter extends CustomPainter {
  _CorePainter(this.pos) : super(repaint: pos);
  final ValueNotifier<Offset> pos;

  static const _colors = [
    Color(0xF5180602),
    Color(0xEB4E1004),
    Color(0xD1A02C0A),
    Color(0x9ED4641A),
    Color(0x5CE4A034),
    Color(0x00E8D6B8),
  ];
  static const _stops = [0.0, .13, .28, .44, .59, .76];

  @override
  void paint(Canvas canvas, Size size) {
    final s = math.max(size.width, size.height) * 1.2;
    final c = Offset(size.width * pos.value.dx, size.height * pos.value.dy);
    canvas.save();
    canvas.translate(c.dx, c.dy);
    canvas.rotate(-24 * math.pi / 180);
    canvas.scale(.82 * s / 2, 1.18 * s / 2);
    canvas.drawCircle(Offset.zero, 1, Paint()..shader = const RadialGradient(colors: _colors, stops: _stops)
        .createShader(Rect.fromCircle(center: Offset.zero, radius: 1)));
    canvas.restore();
  }

  @override
  bool shouldRepaint(_CorePainter old) => false;
}
