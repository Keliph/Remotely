// Shared look: warm blurred background + frosted glass panels (matches the remote page).
import 'dart:ui' show ImageFilter;

import 'package:flutter/material.dart';

const Color ink = Colors.white;
const Color ink2 = Color(0xC7FFFFFF); // 78%
const Color ink3 = Color(0x85FFFFFF); // 52%
const Color pill = Color(0x4D1C1816); // smoked glass button

class WarmBackground extends StatelessWidget {
  const WarmBackground({super.key, required this.child});
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [Color(0xFF2A1C16), Color(0xFF4A4240), Color(0xFF6F7275)],
          stops: [0, .52, 1],
        ),
      ),
      child: DecoratedBox(
        decoration: const BoxDecoration(
          gradient: RadialGradient(
            center: Alignment(-0.5, -0.55),
            radius: 0.95,
            colors: [Color(0xB3E3961E), Color(0x40CF5A1A), Color(0x00E3961E)],
            stops: [0, .45, 1],
          ),
        ),
        child: child,
      ),
    );
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
