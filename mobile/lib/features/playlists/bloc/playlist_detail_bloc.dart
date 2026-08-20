import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../core/api/api_client.dart';
import '../models/playlist_model.dart';
import '../repositories/playlist_repository.dart';

part 'playlist_detail_event.dart';
part 'playlist_detail_state.dart';

/// Drives a single playlist screen: load it, add/remove tracks, and reorder
/// the queue by drag-and-drop.
class PlaylistDetailBloc extends Bloc<PlaylistDetailEvent, PlaylistDetailState> {
  PlaylistDetailBloc({required PlaylistRepository repository, required String playlistId})
      : _repository = repository,
        _playlistId = playlistId,
        super(const PlaylistDetailLoading()) {
    on<PlaylistDetailRequested>(_onRequested);
    on<TrackAdded>(_onTrackAdded);
    on<TrackRemoved>(_onTrackRemoved);
    on<TracksReordered>(_onReordered);
  }

  final PlaylistRepository _repository;
  final String _playlistId;

  Future<void> _onRequested(PlaylistDetailRequested event, Emitter<PlaylistDetailState> emit) async {
    emit(const PlaylistDetailLoading());
    try {
      emit(PlaylistDetailLoaded(await _repository.get(_playlistId)));
    } on ApiException catch (e) {
      emit(PlaylistDetailFailure(e.message));
    }
  }

  Future<void> _onTrackAdded(TrackAdded event, Emitter<PlaylistDetailState> emit) async {
    try {
      await _repository.addTrack(_playlistId, title: event.title, artist: event.artist);
      emit(PlaylistDetailLoaded(await _repository.get(_playlistId)));
    } on ApiException catch (e) {
      emit(PlaylistDetailFailure(e.message));
    }
  }

  Future<void> _onTrackRemoved(TrackRemoved event, Emitter<PlaylistDetailState> emit) async {
    try {
      await _repository.removeTrack(_playlistId, event.trackId);
      emit(PlaylistDetailLoaded(await _repository.get(_playlistId)));
    } on ApiException catch (e) {
      emit(PlaylistDetailFailure(e.message));
    }
  }

  /// Optimistically reorder locally for instant feedback, then persist. If the
  /// server rejects it, surface the error and reload the canonical order.
  Future<void> _onReordered(TracksReordered event, Emitter<PlaylistDetailState> emit) async {
    final current = state;
    if (current is! PlaylistDetailLoaded) return;

    final tracks = List<TrackModel>.of(current.playlist.tracks);
    if (event.oldIndex < 0 || event.oldIndex >= tracks.length) return;
    final moved = tracks.removeAt(event.oldIndex);
    var target = event.newIndex;
    if (target < 0) target = 0;
    if (target > tracks.length) target = tracks.length;
    tracks.insert(target, moved);

    emit(PlaylistDetailLoaded(current.playlist.copyWith(tracks: tracks)));

    try {
      final saved = await _repository.reorder(_playlistId, tracks.map((t) => t.id).toList());
      emit(PlaylistDetailLoaded(saved));
    } on ApiException catch (e) {
      emit(PlaylistDetailFailure(e.message));
      try {
        emit(PlaylistDetailLoaded(await _repository.get(_playlistId)));
      } on ApiException {
        // Keep the failure state if we can't even reload.
      }
    }
  }
}
