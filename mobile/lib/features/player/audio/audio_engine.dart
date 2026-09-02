/// What the player needs from an audio backend, and nothing more.
///
/// Why an interface instead of using `just_audio`'s `AudioPlayer` directly:
/// `AudioPlayer` reaches the platform through method channels, so a bloc that
/// depends on it can only be tested in an integration/widget harness with a
/// real engine behind it. Depending on this port instead means the whole
/// playback state machine — including interruption handling, which is the
/// hard part to reproduce on a device — is covered by fast unit tests, and
/// `just_audio` stays an implementation detail in one file
/// ([JustAudioEngine]).
library;

/// Coarse playback state, deliberately smaller than just_audio's
/// ProcessingState × playing matrix: the UI only distinguishes these.
enum PlaybackStatus { idle, loading, playing, paused, completed }

/// Why playback was interrupted, as reported by the OS audio session.
enum InterruptionKind {
  /// Something else took the audio focus (an incoming call, a voice
  /// assistant). Playback should pause.
  begin,

  /// The interruption ended. [shouldResume] says whether the OS expects us
  /// to start again — after a phone call it does; after e.g. a permanent
  /// focus loss it does not.
  end,
}

class InterruptionEvent {
  const InterruptionEvent(this.kind, {this.shouldResume = false});

  final InterruptionKind kind;
  final bool shouldResume;
}

/// A playback backend.
abstract class AudioEngine {
  /// Coarse status updates.
  Stream<PlaybackStatus> get statusStream;

  /// Playback position, for the seek bar.
  Stream<Duration> get positionStream;

  /// Total duration, or null for a live stream (which has no end).
  Stream<Duration?> get durationStream;

  /// OS-level interruptions (incoming call, another app taking focus).
  Stream<InterruptionEvent> get interruptionStream;

  double get volume;

  /// Points the engine at [url]. [isLive] tells it not to expect a duration.
  Future<void> setSource(String url, {required bool isLive});

  Future<void> play();
  Future<void> pause();
  Future<void> stop();
  Future<void> seek(Duration position);
  Future<void> setVolume(double volume);
  Future<void> dispose();
}
