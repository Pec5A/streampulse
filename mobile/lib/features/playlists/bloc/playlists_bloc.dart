import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../core/api/api_client.dart';
import '../models/playlist_model.dart';
import '../repositories/playlist_repository.dart';

part 'playlists_event.dart';
part 'playlists_state.dart';

/// Drives the playlists list screen: load the caller's playlists and create
/// new ones.
class PlaylistsBloc extends Bloc<PlaylistsEvent, PlaylistsState> {
  PlaylistsBloc({required PlaylistRepository repository})
      : _repository = repository,
        super(const PlaylistsInitial()) {
    on<PlaylistsRequested>(_onRequested);
    on<PlaylistCreated>(_onCreated);
  }

  final PlaylistRepository _repository;

  Future<void> _onRequested(PlaylistsRequested event, Emitter<PlaylistsState> emit) async {
    emit(const PlaylistsLoading());
    try {
      emit(PlaylistsLoaded(await _repository.list()));
    } on ApiException catch (e) {
      emit(PlaylistsFailure(e.message));
    }
  }

  Future<void> _onCreated(PlaylistCreated event, Emitter<PlaylistsState> emit) async {
    try {
      await _repository.create(event.name, description: event.description);
      emit(PlaylistsLoaded(await _repository.list()));
    } on ApiException catch (e) {
      emit(PlaylistsFailure(e.message));
    }
  }
}
