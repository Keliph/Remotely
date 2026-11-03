// Shared look: warm blurred background + frosted glass panels (matches the remote page).
import 'dart:math' as math;
import 'dart:ui' as ui show Gradient;
import 'dart:ui' show ImageFilter;

import 'package:flutter/material.dart';

const Color ink = Colors.white;
const Color ink2 = Color(0xC7FFFFFF); // 78%
const Color ink3 = Color(0x85FFFFFF); // 52%
const Color pill = Color(0x4D1C1816); // smoked glass button
const Color pillPress = Color(0x851C1816);
const Color solidInk = Color(0xFF1D1410); // text on the one white button

// ---- CSS-style gradient layers, so the app paints exactly what the remote page's CSS paints ----

abstract class PaintLayer {
  const PaintLayer();
  void paint(Canvas canvas, Size size);
}

/// CSS `linear-gradient(<deg>, ...)`.
class LinearLayer extends PaintLayer {
  const LinearLayer(this.degrees, this.colors, this.stops);
  final double degrees;
  final List<Color> colors;
  final List<double> stops;

  @override
  void paint(Canvas canvas, Size size) {
    final a = degrees * math.pi / 180;
    final dir = Offset(math.sin(a), -math.cos(a));
    final half = (size.width * math.sin(a).abs() + size.height * math.cos(a).abs()) / 2;
    final c = size.center(Offset.zero);
    canvas.drawRect(Offset.zero & size, Paint()..shader = ui.Gradient.linear(c - dir * half, c + dir * half, colors, stops));
  }
}

/// CSS `radial-gradient(<rx%> <ry%> at <x%> <y%>, ...)`: an elliptical glow.
class GlowLayer extends PaintLayer {
  const GlowLayer({required this.at, required this.radii, required this.colors, required this.stops});
  final Offset at; // fraction of width / height
  final Size radii; // fraction of width / height
  final List<Color> colors;
  final List<double> stops;

  @override
  void paint(Canvas canvas, Size size) {
    final c = Offset(size.width * at.dx, size.height * at.dy);
    final sx = size.width * radii.width, sy = size.height * radii.height;
    canvas.save();
    canvas.clipRect(Offset.zero & size);
    canvas.translate(c.dx, c.dy);
    canvas.scale(sx, sy);
    canvas.drawRect(Rect.fromLTWH(-c.dx / sx, -c.dy / sy, size.width / sx, size.height / sy),
        Paint()..shader = ui.Gradient.radial(Offset.zero, 1, colors, stops));
    canvas.restore();
  }
}

class LayersPainter extends CustomPainter {
  const LayersPainter(this.layers);
  final List<PaintLayer> layers;

  @override
  void paint(Canvas canvas, Size size) {
    for (final l in layers) {
      l.paint(canvas, size);
    }
  }

  @override
  bool shouldRepaint(LayersPainter old) => old.layers != layers;
}

/// The remote page's background (body in remote.html), bottom layer first.
const remoteBackground = <PaintLayer>[
  LinearLayer(180, [Color(0xFF2A1C16), Color(0xFF4A4240), Color(0xFF6F7275)], [0, .52, 1]),
  GlowLayer(at: Offset(.90, .92), radii: Size(.60, .35), colors: [Color(0x66A0ACB8), Color(0x00A0ACB8)], stops: [0, .70]),
  GlowLayer(at: Offset(.32, .64), radii: Size(.75, .32), colors: [Color(0x80E3961E), Color(0x00E3961E)], stops: [0, .72]),
];

/// The touchpad card (#pad and #pad::before in remote.html).
const padLayers = <PaintLayer>[
  LinearLayer(150, [Color(0xFFEF9A52), Color(0xFFE8B878), Color(0xFFE9D7B8), Color(0xFFDDC9A4)], [0, .26, .58, 1]),
  LinearLayer(90, [Color(0xA6EC6822), Color(0x00EC6822)], [0, .26]),
  GlowLayer(at: Offset(.30, 1), radii: Size(.70, .45), colors: [Color(0xE6E8A018), Color(0x00E8A018)], stops: [0, .75]),
];

class WarmBackground extends StatelessWidget {
  const WarmBackground({super.key, required this.child});
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return CustomPaint(painter: const LayersPainter(remoteBackground), child: child);
  }
}

class Glass extends StatelessWidget {
  const Glass({
    super.key,
    required this.child,
    this.radius = 30,
    this.padding = const EdgeInsets.all(20),
    this.onTap,
    this.onLongPress,
  });

  final Widget child;
  final double radius;
  final EdgeInsetsGeometry padding;
  final VoidCallback? onTap;
  final VoidCallback? onLongPress;

  @override
  Widget build(BuildContext context) {
    final shape = BorderRadius.circular(radius);
    return ClipRRect(
      borderRadius: shape,
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 24, sigmaY: 24),
        child: Material(
          color: const Color(0x21FFFFFF),
          shape: RoundedRectangleBorder(
            borderRadius: shape,
            side: const BorderSide(color: Color(0x29FFFFFF)),
          ),
          child: InkWell(
            onTap: onTap,
            onLongPress: onLongPress,
            borderRadius: shape,
            splashColor: const Color(0x1FFFFFFF),
            highlightColor: const Color(0x14FFFFFF),
            child: Padding(padding: padding, child: child),
          ),
        ),
      ),
    );
  }
}

class PillButton extends StatelessWidget {
  const PillButton({super.key, required this.label, required this.onPressed, this.icon, this.solid = false});
  final String label;
  final IconData? icon;
  final VoidCallback? onPressed;
  final bool solid;

  @override
  Widget build(BuildContext context) {
    final fg = solid ? const Color(0xFF1D1410) : ink;
    return Material(
      color: solid ? Colors.white : pill,
      shape: const StadiumBorder(),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onPressed,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 15),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              if (icon != null) ...[Icon(icon, size: 20, color: fg), const SizedBox(width: 8)],
              Text(label, style: TextStyle(color: fg, fontSize: 15, fontWeight: FontWeight.w400)),
            ],
          ),
        ),
      ),
    );
  }
}
