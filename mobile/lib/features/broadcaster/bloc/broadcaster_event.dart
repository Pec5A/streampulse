part of 'broadcaster_bloc.dart';

sealed class BroadcasterEvent extends Equatable {
  const BroadcasterEvent();
  @override
  List<Object?> get props => [];
}

final class BroadcasterTracksRequested extends BroadcasterEvent {
  const BroadcasterTracksRequested();
}

/// Opens the device file picker.
final class BroadcasterTrackPicked extends BroadcasterEvent {
  const BroadcasterTrackPicked();
}

final class BroadcasterUploadRequested extends BroadcasterEvent {
  const BroadcasterUploadRequested({required this.title, this.artist = ''});
  final String title;
  final String artist;
  @override
  List<Object?> get props => [title, artist];
}

final class BroadcasterTrackDeleted extends BroadcasterEvent {
  const BroadcasterTrackDeleted(this.trackId);
  final String trackId;
  @override
  List<Object?> get props => [trackId];
}

final class BroadcasterStreamCreated extends BroadcasterEvent {
  const BroadcasterStreamCreated({required this.title, this.description = ''});
  final String title;
  final String description;
  @override
  List<Object?> get props => [title, description];
}

final class BroadcasterGoLiveRequested extends BroadcasterEvent {
  const BroadcasterGoLiveRequested(this.track);
  final Track track;
  @override
  List<Object?> get props => [track.id];
}

final class BroadcasterStopRequested extends BroadcasterEvent {
  const BroadcasterStopRequested();
}
