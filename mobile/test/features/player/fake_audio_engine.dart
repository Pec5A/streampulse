import 'dart:async';

import 'package:streampulse/features/player/audio/audio_engine.dart';

/// A hand-written [AudioEngine] double.
///
/// Hand-written rather than a mocktail mock because the interesting part of
/// this port is its *streams*: tests need to push a status change or an
/// interruption at a chosen moment, which is awkward to express with stubbed
/// return values and trivial here.
class FakeAudioEngine implements AudioEngine {
  final statusController = StreamController<PlaybackStatus>.broadcast();
  final positionController = StreamController<Duration>.broadcast();
  final durationController = StreamController<Duration?>.broadcast();
  final interruptionController = StreamController<InterruptionEvent>.broadcast();

  /// Every call, in order — lets a test assert that a pause really reached
  /// the engine and not just the state.
  final calls = <String>[];

  double _volume = 1;
  bool disposed = false;

  /// When set, the next matching call throws it (simulates a dead network).
  Object? throwOnSetSource;
  Object? throwOnPlay;

  @override
  Stream<PlaybackStatus> get statusStream => statusController.stream;

  @override
  Stream<Duration> get positionStream => positionController.stream;

  @override
  Stream<Duration?> get durationStream => durationController.stream;

  @override
  Stream<InterruptionEvent> get interruptionStream => interruptionController.stream;

  @override
  double get volume => _volume;

  @override
  Future<void> setSource(String url, {required bool isLive}) async {
    calls.add('setSource($url, isLive: $isLive)');
    if (throwOnSetSource != null) throw throwOnSetSource!;
  }

  @override
  Future<void> play() async {
    calls.add('play');
    if (throwOnPlay != null) throw throwOnPlay!;
  }

  @override
  Future<void> pause() async => calls.add('pause');

  @override
  Future<void> stop() async => calls.add('stop');

  @override
  Future<void> seek(Duration position) async => calls.add('seek($position)');

  @override
  Future<void> setVolume(double volume) async {
    _volume = volume;
    calls.add('setVolume($volume)');
  }

  @override
  Future<void> dispose() async {
    disposed = true;
    await statusController.close();
    await positionController.close();
    await durationController.close();
    await interruptionController.close();
  }
}
