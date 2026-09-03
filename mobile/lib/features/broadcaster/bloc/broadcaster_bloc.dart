import 'dart:async';

import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:http/http.dart' as http;

import '../../player/models/live_stream.dart';
import '../broadcast/audio_file_picker.dart';
import '../broadcast/broadcast_transport.dart';
import '../models/track.dart';
import '../repositories/broadcaster_repository.dart';

part 'broadcaster_event.dart';
part 'broadcaster_state.dart';

/// Drives the broadcaster screen: manage your tracks, create a stream, and
/// go live with one of your tracks.
///
/// Every platform dependency is behind a port ([AudioFilePicker],
/// [BroadcastTransport]), so this whole state machine — including the part
/// that matters most, "the broadcast stopped, is the stream back offline?" —
/// runs in unit tests without a device or a server.
class BroadcasterBloc extends Bloc<BroadcasterEvent, BroadcasterState> {
  BroadcasterBloc({
    required BroadcasterRepository repository,
    required AudioFilePicker picker,
    required BroadcastTransport transport,
    required String token,
    http.Client? audioClient,
  })  : _repository = repository,
        _picker = picker,
        _transport = transport,
        _token = token,
        _audioClient = audioClient ?? http.Client(),
        super(const BroadcasterState()) {
    on<BroadcasterTracksRequested>(_onTracksRequested);
    on<BroadcasterTrackPicked>(_onTrackPicked);
    on<BroadcasterUploadRequested>(_onUploadRequested);
    on<BroadcasterTrackDeleted>(_onTrackDeleted);
    on<BroadcasterStreamCreated>(_onStreamCreated);
    on<BroadcasterGoLiveRequested>(_onGoLive);
    on<BroadcasterStopRequested>(_onStop);
  }

  final BroadcasterRepository _repository;
  final AudioFilePicker _picker;
  final BroadcastTransport _transport;
  final String _token;
  final http.Client _audioClient;

  Future<void> _onTracksRequested(BroadcasterTracksRequested event, Emitter<BroadcasterState> emit) async {
    emit(state.copyWith(loading: true, clearError: true));
    try {
      final tracks = await _repository.fetchMyTracks(_token);
      emit(state.copyWith(tracks: tracks, loading: false));
    } on Object catch (e) {
      emit(state.copyWith(loading: false, errorMessage: e.toString()));
    }
  }

  Future<void> _onTrackPicked(BroadcasterTrackPicked event, Emitter<BroadcasterState> emit) async {
    try {
      final picked = await _picker.pick();
      // A cancelled picker is not an error — leave the state untouched.
      if (picked == null) return;
      emit(state.copyWith(pendingUpload: picked, clearError: true));
    } on Object catch (e) {
      emit(state.copyWith(errorMessage: e.toString()));
    }
  }

  Future<void> _onUploadRequested(BroadcasterUploadRequested event, Emitter<BroadcasterState> emit) async {
    final pending = state.pendingUpload;
    if (pending == null) {
      emit(state.copyWith(errorMessage: 'Choisis un fichier audio avant de téléverser.'));
      return;
    }
    if (event.title.trim().isEmpty) {
      emit(state.copyWith(errorMessage: 'Le titre est obligatoire.'));
      return;
    }

    emit(state.copyWith(uploading: true, clearError: true));
    try {
      final track = await _repository.uploadTrack(
        token: _token,
        title: event.title.trim(),
        artist: event.artist.trim(),
        audio: pending,
      );
      emit(state.copyWith(
        tracks: [track, ...state.tracks],
        uploading: false,
        clearPendingUpload: true,
      ));
    } on Object catch (e) {
      emit(state.copyWith(uploading: false, errorMessage: e.toString()));
    }
  }

  Future<void> _onTrackDeleted(BroadcasterTrackDeleted event, Emitter<BroadcasterState> emit) async {
    try {
      await _repository.deleteTrack(token: _token, trackId: event.trackId);
      emit(state.copyWith(
        tracks: state.tracks.where((t) => t.id != event.trackId).toList(),
        clearError: true,
      ));
    } on Object catch (e) {
      emit(state.copyWith(errorMessage: e.toString()));
    }
  }

  Future<void> _onStreamCreated(BroadcasterStreamCreated event, Emitter<BroadcasterState> emit) async {
    if (event.title.trim().isEmpty) {
      emit(state.copyWith(errorMessage: 'Le titre du direct est obligatoire.'));
      return;
    }

    emit(state.copyWith(loading: true, clearError: true));
    try {
      final stream = await _repository.createStream(
        token: _token,
        title: event.title.trim(),
        description: event.description.trim(),
      );
      emit(state.copyWith(stream: stream, loading: false));
    } on Object catch (e) {
      emit(state.copyWith(loading: false, errorMessage: e.toString()));
    }
  }

  Future<void> _onGoLive(BroadcasterGoLiveRequested event, Emitter<BroadcasterState> emit) async {
    final stream = state.stream;
    if (stream == null) {
      emit(state.copyWith(errorMessage: 'Crée un direct avant de diffuser.'));
      return;
    }
    emit(state.copyWith(starting: true, clearError: true));

    try {
      final response = await _audioClient.send(
        http.Request('GET', Uri.parse(_repository.trackAudioUrl(event.track))),
      );
      if (response.statusCode != 200) {
        throw StateError('audio introuvable (${response.statusCode})');
      }

      // Paced, otherwise the whole file would be pushed in seconds and the
      // "live" broadcast would be over before anyone tuned in.
      final source = pacedSource(response.stream);

      if (_transport.isBroadcasting) {
        // Already on air: swap what is being sent through the connection that
        // is already open. Stopping and restarting would flip the stream
        // offline and end every listener's response — the audience would be
        // disconnected by a change of record.
        await _transport.switchSource(source);
      } else {
        await _transport.start(
          publishUrl: _repository.publishUrl(stream.id),
          token: _token,
          source: source,
        );
      }
      emit(state.copyWith(starting: false, isLive: true, broadcastingTrack: event.track));
    } on Object catch (e) {
      // Leave no half-open broadcast behind if the source failed.
      await _transport.stop();
      emit(state.copyWith(starting: false, isLive: false, errorMessage: e.toString()));
    }
  }

  Future<void> _onStop(BroadcasterStopRequested event, Emitter<BroadcasterState> emit) async {
    await _transport.stop();
    emit(state.copyWith(isLive: false, clearBroadcastingTrack: true));
  }

  @override
  Future<void> close() async {
    await _transport.stop();
    _audioClient.close();
    return super.close();
  }
}
