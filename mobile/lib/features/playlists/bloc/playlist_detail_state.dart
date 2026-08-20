part of 'playlist_detail_bloc.dart';

sealed class PlaylistDetailState extends Equatable {
  const PlaylistDetailState();
  @override
  List<Object?> get props => [];
}

final class PlaylistDetailLoading extends PlaylistDetailState {
  const PlaylistDetailLoading();
}

final class PlaylistDetailLoaded extends PlaylistDetailState {
  const PlaylistDetailLoaded(this.playlist);
  final PlaylistModel playlist;
  @override
  List<Object?> get props => [playlist];
}

final class PlaylistDetailFailure extends PlaylistDetailState {
  const PlaylistDetailFailure(this.message);
  final String message;
  @override
  List<Object?> get props => [message];
}
