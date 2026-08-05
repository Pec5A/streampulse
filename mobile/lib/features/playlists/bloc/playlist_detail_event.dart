part of 'playlist_detail_bloc.dart';

sealed class PlaylistDetailEvent extends Equatable {
  const PlaylistDetailEvent();
  @override
  List<Object?> get props => [];
}

/// Load (or reload) the playlist and its tracks.
final class PlaylistDetailRequested extends PlaylistDetailEvent {
  const PlaylistDetailRequested();
}

final class TrackAdded extends PlaylistDetailEvent {
  const TrackAdded({required this.title, this.artist = ''});
  final String title;
  final String artist;
  @override
  List<Object?> get props => [title, artist];
}

final class TrackRemoved extends PlaylistDetailEvent {
  const TrackRemoved(this.trackId);
  final String trackId;
  @override
  List<Object?> get props => [trackId];
}

/// [newIndex] is the target index *after* [oldIndex] has been removed (the
/// screen adjusts for ReorderableListView's index convention before dispatch).
final class TracksReordered extends PlaylistDetailEvent {
  const TracksReordered({required this.oldIndex, required this.newIndex});
  final int oldIndex;
  final int newIndex;
  @override
  List<Object?> get props => [oldIndex, newIndex];
}
