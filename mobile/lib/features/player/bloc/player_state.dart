part of 'player_bloc.dart';

/// One flat state rather than a sealed hierarchy: every field here (volume,
/// position, the selected stream) survives a status change, so modelling
/// "playing" and "paused" as separate classes would mean copying the same
/// six fields between them on every transition.
class PlayerStateData extends Equatable {
  const PlayerStateData({
    this.stream,
    this.status = PlaybackStatus.idle,
    this.position = Duration.zero,
    this.duration,
    this.volume = 1.0,
    this.errorMessage,
    this.interrupted = false,
  });

  /// The stream currently loaded, or null when nothing is selected.
  final LiveStream? stream;
  final PlaybackStatus status;
  final Duration position;

  /// Null for a live broadcast — it has no end.
  final Duration? duration;
  final double volume;
  final String? errorMessage;

  /// True while the OS has taken audio focus (an incoming call). The UI
  /// shows this rather than looking like the user paused it themselves.
  final bool interrupted;

  bool get isPlaying => status == PlaybackStatus.playing;
  bool get isLoading => status == PlaybackStatus.loading;
  bool get hasStream => stream != null;

  /// Seeking only makes sense on something with a known length. A live
  /// broadcast has no past to scrub back into, so the UI disables the bar.
  bool get canSeek => duration != null && duration! > Duration.zero;

  PlayerStateData copyWith({
    LiveStream? stream,
    PlaybackStatus? status,
    Duration? position,
    Duration? duration,
    double? volume,
    String? errorMessage,
    bool? interrupted,
    bool clearError = false,
    bool clearDuration = false,
  }) {
    return PlayerStateData(
      stream: stream ?? this.stream,
      status: status ?? this.status,
      position: position ?? this.position,
      duration: clearDuration ? null : (duration ?? this.duration),
      volume: volume ?? this.volume,
      errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
      interrupted: interrupted ?? this.interrupted,
    );
  }

  @override
  List<Object?> get props => [stream?.id, status, position, duration, volume, errorMessage, interrupted];
}
