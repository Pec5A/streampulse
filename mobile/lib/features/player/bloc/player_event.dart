part of 'player_bloc.dart';

sealed class PlayerEvent extends Equatable {
  const PlayerEvent();
  @override
  List<Object?> get props => [];
}

final class PlayerStreamSelected extends PlayerEvent {
  const PlayerStreamSelected({required this.stream, required this.url});
  final LiveStream stream;
  final String url;
  @override
  List<Object?> get props => [stream.id, url];
}

final class PlayerPlayRequested extends PlayerEvent {
  const PlayerPlayRequested();
}

final class PlayerPauseRequested extends PlayerEvent {
  const PlayerPauseRequested();
}

/// Single control for the play/pause button, so the UI never has to work out
/// which of the two to dispatch.
final class PlayerPlayPauseToggled extends PlayerEvent {
  const PlayerPlayPauseToggled();
}

final class PlayerStopRequested extends PlayerEvent {
  const PlayerStopRequested();
}

final class PlayerVolumeChanged extends PlayerEvent {
  const PlayerVolumeChanged(this.volume);
  final double volume;
  @override
  List<Object?> get props => [volume];
}

final class PlayerSeekRequested extends PlayerEvent {
  const PlayerSeekRequested(this.position);
  final Duration position;
  @override
  List<Object?> get props => [position];
}

/// Emitted by the engine, not the UI.
final class PlayerEngineStatusChanged extends PlayerEvent {
  const PlayerEngineStatusChanged(this.status);
  final PlaybackStatus status;
  @override
  List<Object?> get props => [status];
}

final class PlayerPositionChanged extends PlayerEvent {
  const PlayerPositionChanged(this.position);
  final Duration position;
  @override
  List<Object?> get props => [position];
}

final class PlayerDurationChanged extends PlayerEvent {
  const PlayerDurationChanged(this.duration);
  final Duration? duration;
  @override
  List<Object?> get props => [duration];
}

final class PlayerInterrupted extends PlayerEvent {
  const PlayerInterrupted(this.event);
  final InterruptionEvent event;
  @override
  List<Object?> get props => [event.kind, event.shouldResume];
}
