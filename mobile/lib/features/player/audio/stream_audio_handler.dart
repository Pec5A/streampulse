import 'package:audio_service/audio_service.dart';

import '../models/live_stream.dart';
import 'audio_engine.dart';

/// Bridges the player to `audio_service`, which is what keeps audio running
/// when the app is backgrounded and puts controls on the lock screen and in
/// the notification shade.
///
/// It is one object playing two roles on purpose: `audio_service` needs a
/// [BaseAudioHandler] to drive from the OS side, and [PlayerBloc] needs an
/// [AudioEngine] to drive from the UI side. Wrapping the same underlying
/// engine in both interfaces means a pause from the lock screen and a pause
/// from the app take exactly the same path — there is no second state
/// machine to keep in sync.
class StreamAudioHandler extends BaseAudioHandler implements AudioEngine {
  StreamAudioHandler(this._engine) {
    _engine.statusStream.listen(_publishState);
  }

  final AudioEngine _engine;

  /// Publishes the current status to the OS so the notification shows the
  /// right button and the lock screen stays in sync.
  void _publishState(PlaybackStatus status) {
    playbackState.add(playbackState.value.copyWith(
      controls: [
        if (status == PlaybackStatus.playing) MediaControl.pause else MediaControl.play,
        MediaControl.stop,
      ],
      systemActions: const {MediaAction.seek},
      processingState: switch (status) {
        PlaybackStatus.idle => AudioProcessingState.idle,
        PlaybackStatus.loading => AudioProcessingState.loading,
        PlaybackStatus.completed => AudioProcessingState.completed,
        PlaybackStatus.playing || PlaybackStatus.paused => AudioProcessingState.ready,
      },
      playing: status == PlaybackStatus.playing,
    ));
  }

  /// Names the broadcast in the notification and on the lock screen.
  void describe(LiveStream stream) {
    mediaItem.add(MediaItem(
      id: stream.id,
      title: stream.title,
      artist: stream.broadcasterUsername.isEmpty ? 'StreamPulse' : stream.broadcasterUsername,
      // A live broadcast has no duration; audio_service renders a live
      // indicator instead of a progress bar when this is null.
      duration: null,
      isLive: stream.isLive,
    ));
  }

  // --- AudioEngine (the app side) -----------------------------------------

  @override
  Stream<PlaybackStatus> get statusStream => _engine.statusStream;

  @override
  Stream<Duration> get positionStream => _engine.positionStream;

  @override
  Stream<Duration?> get durationStream => _engine.durationStream;

  @override
  Stream<InterruptionEvent> get interruptionStream => _engine.interruptionStream;

  @override
  double get volume => _engine.volume;

  @override
  Future<void> setSource(String url, {required bool isLive}) =>
      _engine.setSource(url, isLive: isLive);

  @override
  Future<void> setVolume(double volume) => _engine.setVolume(volume);

  @override
  Future<void> dispose() => _engine.dispose();

  // --- BaseAudioHandler (the OS side) -------------------------------------
  //
  // These four are the shared surface: the lock screen calls them, and so
  // does the bloc through the AudioEngine interface.

  @override
  Future<void> play() => _engine.play();

  @override
  Future<void> pause() => _engine.pause();

  @override
  Future<void> stop() async {
    await _engine.stop();
    await super.stop();
  }

  @override
  Future<void> seek(Duration position) => _engine.seek(position);
}
