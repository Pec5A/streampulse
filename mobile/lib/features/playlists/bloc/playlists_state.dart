part of 'playlists_bloc.dart';

sealed class PlaylistsState extends Equatable {
  const PlaylistsState();
  @override
  List<Object?> get props => [];
}

final class PlaylistsInitial extends PlaylistsState {
  const PlaylistsInitial();
}

final class PlaylistsLoading extends PlaylistsState {
  const PlaylistsLoading();
}

final class PlaylistsLoaded extends PlaylistsState {
  const PlaylistsLoaded(this.playlists);
  final List<PlaylistModel> playlists;
  @override
  List<Object?> get props => [playlists];
}

final class PlaylistsFailure extends PlaylistsState {
  const PlaylistsFailure(this.message);
  final String message;
  @override
  List<Object?> get props => [message];
}
