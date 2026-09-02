import 'dart:async';

import 'package:audio_session/audio_session.dart';
import 'package:just_audio/just_audio.dart';

import 'audio_engine.dart';

/// The real [AudioEngine]: `just_audio` for playback, `audio_session` for the
/// OS audio session (routing, ducking, interruptions).
///
/// This is the only file in the app that imports either package, so swapping
/// the backend — or testing everything above it — costs nothing.
class JustAudioEngine implements AudioEngine {
  JustAudioEngine({AudioPlayer? player}) : _player = player ?? AudioPlayer();

  final AudioPlayer _player;
  final _interruptions = StreamController<InterruptionEvent>.broadcast();

  AudioSession? _session;
  bool _wasPlayingBeforeInterruption = false;
  double _volumeBeforeDuck = 1;

  /// Configures the OS audio session. Must be awaited once before playing:
  /// without it, Android will not grant audio focus and iOS will not keep
  /// playing when the screen locks.
  Future<void> configure() async {
    final session = await AudioSession.instance;
    await session.configure(const AudioSessionConfiguration.music());
    _session = session;

    session.interruptionEventStream.listen(_onInterruption);

    // Unplugging headphones must pause rather than blast audio out of the
    // speaker — the OS reports this separately from interruptions.
    session.becomingNoisyEventStream.listen((_) {
      _interruptions.add(const InterruptionEvent(InterruptionKind.begin));
    });
  }

  Future<void> _onInterruption(AudioInterruptionEvent event) async {
    if (event.begin) {
      switch (event.type) {
        case AudioInterruptionType.duck:
          // Something short (a navigation prompt): stay audible, quieter.
          _volumeBeforeDuck = _player.volume;
          await _player.setVolume(_volumeBeforeDuck / 2);
        case AudioInterruptionType.pause:
        case AudioInterruptionType.unknown:
          _wasPlayingBeforeInterruption = _player.playing;
          _interruptions.add(const InterruptionEvent(InterruptionKind.begin));
      }
      return;
    }

    switch (event.type) {
      case AudioInterruptionType.duck:
        await _player.setVolume(_volumeBeforeDuck);
      case AudioInterruptionType.pause:
        // Resume only if we were actually playing when the call came in.
        _interruptions.add(InterruptionEvent(
          InterruptionKind.end,
          shouldResume: _wasPlayingBeforeInterruption,
        ));
      case AudioInterruptionType.unknown:
        // The OS does not expect us to come back on our own here.
        _interruptions.add(const InterruptionEvent(InterruptionKind.end));
    }
  }

  @override
  Stream<PlaybackStatus> get statusStream => _player.playerStateStream.map(_toStatus).distinct();

  static PlaybackStatus _toStatus(PlayerState state) {
    switch (state.processingState) {
      case ProcessingState.idle:
        return PlaybackStatus.idle;
      case ProcessingState.loading:
      case ProcessingState.buffering:
        return PlaybackStatus.loading;
      case ProcessingState.completed:
        return PlaybackStatus.completed;
      case ProcessingState.ready:
        return state.playing ? PlaybackStatus.playing : PlaybackStatus.paused;
    }
  }

  @override
  Stream<Duration> get positionStream => _player.positionStream;

  @override
  Stream<Duration?> get durationStream => _player.durationStream;

  @override
  Stream<InterruptionEvent> get interruptionStream => _interruptions.stream;

  @override
  double get volume => _player.volume;

  @override
  Future<void> setSource(String url, {required bool isLive}) async {
    // A live broadcast is an endless chunked response: there is nothing to
    // preload and no duration to discover, so we do not wait on the future
    // just_audio returns for a finite track.
    await _player.setUrl(url);
  }

  @override
  Future<void> play() => _player.play();

  @override
  Future<void> pause() => _player.pause();

  @override
  Future<void> stop() => _player.stop();

  @override
  Future<void> seek(Duration position) => _player.seek(position);

  @override
  Future<void> setVolume(double volume) => _player.setVolume(volume.clamp(0.0, 1.0));

  @override
  Future<void> dispose() async {
    await _interruptions.close();
    await _session?.setActive(false);
    await _player.dispose();
  }
}
