part of 'playlists_bloc.dart';

sealed class PlaylistsEvent extends Equatable {
  const PlaylistsEvent();
  @override
  List<Object?> get props => [];
}

/// Load (or reload) the current user's playlists.
final class PlaylistsRequested extends PlaylistsEvent {
  const PlaylistsRequested();
}

/// Create a playlist, then refresh the list.
final class PlaylistCreated extends PlaylistsEvent {
  const PlaylistCreated({required this.name, this.description = ''});
  final String name;
  final String description;
  @override
  List<Object?> get props => [name, description];
}
