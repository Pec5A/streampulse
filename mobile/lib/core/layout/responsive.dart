import 'package:flutter/widgets.dart';

/// Responsive breakpoints for adapting layout between phone and tablet.
abstract final class Breakpoints {
  static const double tablet = 600;
  static const double maxContentWidth = 900;
}

extension ResponsiveContext on BuildContext {
  bool get isTablet => MediaQuery.sizeOf(this).width >= Breakpoints.tablet;
}

/// Centers and width-caps its child on large screens so content (and line
/// length) stays comfortable and readable on tablets.
class ContentWidth extends StatelessWidget {
  const ContentWidth({super.key, required this.child, this.maxWidth = Breakpoints.maxContentWidth});

  final Widget child;
  final double maxWidth;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxWidth),
        child: child,
      ),
    );
  }
}
