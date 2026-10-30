import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'home_screen.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  SystemChrome.setSystemUIOverlayStyle(const SystemUiOverlayStyle(
    statusBarColor: Colors.transparent,
    statusBarIconBrightness: Brightness.light,
    systemNavigationBarColor: Color(0xFF6F7275),
    systemNavigationBarIconBrightness: Brightness.light,
  ));
  runApp(const BlanketRemoteApp());
}

class BlanketRemoteApp extends StatelessWidget {
  const BlanketRemoteApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Blanket Remote',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        brightness: Brightness.dark,
        fontFamily: 'Outfit',
        scaffoldBackgroundColor: const Color(0xFF2A1C16),
        colorScheme: const ColorScheme.dark(
          primary: Colors.white,
          onPrimary: Color(0xFF1D1410),
          secondary: Color(0xFFE3962A),
          surface: Color(0xFF3B3330),
          onSurface: Colors.white,
        ),
        snackBarTheme: const SnackBarThemeData(
          behavior: SnackBarBehavior.floating,
          backgroundColor: Color(0xFF3B3330),
          contentTextStyle: TextStyle(fontFamily: 'Outfit', color: Colors.white, fontSize: 15),
        ),
      ),
      home: const HomeScreen(),
    );
  }
}
