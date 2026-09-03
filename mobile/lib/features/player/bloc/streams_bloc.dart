import 'dart:async';

import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../models/live_stream.dart';
import '../repositories/stream_repository.dart';

sealed class StreamsEvent extends Equatable {
  const StreamsEvent();
  @override
  List<Object?> get props => [];
}

/// Loads the catalogue. [liveOnly] switches between "what is on air right
/// now" and the full list.
final class StreamsRequested extends StreamsEvent {
  const StreamsRequested({this.liveOnly = true});
  final bool liveOnly;
  @override
  List<Object?> get props => [liveOnly];
}

/// Re-reads the catalogue without disturbing what is already on screen.
///
/// Separate from [StreamsRequested] because that one announces itself with
/// [StreamsLoading]: polling with it would blank the list into a spinner
/// every few seconds. This event only ever replaces the content.
final class StreamsRefreshRequested extends StreamsEvent {
  const StreamsRefreshRequested();
}

sealed class StreamsState extends Equatable {
  const StreamsState();
  @override
  List<Object?> get props => [];
}

final class StreamsInitial extends StreamsState {
  const StreamsInitial();
}

final class StreamsLoading extends StreamsState {
  const StreamsLoading();
}

final class StreamsLoaded extends StreamsState {
  const StreamsLoaded({required this.streams, required this.liveOnly});
  final List<LiveStream> streams;
  final bool liveOnly;
  @override
  List<Object?> get props => [streams, liveOnly];
}

final class StreamsFailure extends StreamsState {
  const StreamsFailure({required this.message});
  final String message;
  @override
  List<Object?> get props => [message];
}

/// Keeps the list of broadcasts current.
///
/// It polls, because nothing tells the app that somebody started
/// broadcasting: the API offers no push channel for the catalogue. Without a
/// timer, a stream that goes live on another device stays invisible until the
/// user happens to pull to refresh — which is a poor way for an app about
/// live audio to behave.
class StreamsBloc extends Bloc<StreamsEvent, StreamsState> {
  StreamsBloc({
    required StreamRepository repository,
    this.pollInterval = const Duration(seconds: 5),
  })  : _repository = repository,
        super(const StreamsInitial()) {
    on<StreamsRequested>(_onRequested);
    on<StreamsRefreshRequested>(_onRefreshRequested);

    if (pollInterval > Duration.zero) {
      _poll = Timer.periodic(pollInterval, (_) => add(const StreamsRefreshRequested()));
    }
  }

  /// How often the catalogue is re-read. [Duration.zero] disables polling,
  /// which is what tests of the one-shot behaviour use.
  final Duration pollInterval;

  final StreamRepository _repository;

  Timer? _poll;

  /// The mode of the last explicit request, so a poll refreshes the list the
  /// user is actually looking at rather than silently switching it.
  bool _liveOnly = true;

  /// Guards against overlapping requests: bloc processes events concurrently,
  /// so an API slower than [pollInterval] would otherwise stack up calls.
  bool _fetching = false;

  Future<void> _onRequested(StreamsRequested event, Emitter<StreamsState> emit) async {
    _liveOnly = event.liveOnly;
    emit(const StreamsLoading());
    _fetching = true;
    try {
      emit(StreamsLoaded(streams: await _fetch(), liveOnly: _liveOnly));
    } on Object catch (e) {
      // A failure must be visible: showing an empty list would read as
      // "nobody is broadcasting" when the API is actually down.
      emit(StreamsFailure(message: e.toString()));
    } finally {
      _fetching = false;
    }
  }

  Future<void> _onRefreshRequested(StreamsRefreshRequested event, Emitter<StreamsState> emit) async {
    if (_fetching) return; // a request is already in flight; let it land
    _fetching = true;
    try {
      final streams = await _fetch();
      if (!emit.isDone) emit(StreamsLoaded(streams: streams, liveOnly: _liveOnly));
    } on Object catch (e) {
      // A poll that fails must not replace a good list with an error page:
      // the next tick will most likely succeed, and flickering to "impossible
      // de charger" is worse than a list a few seconds stale. Only speak up
      // when there is nothing worth preserving.
      if (state is! StreamsLoaded && !emit.isDone) {
        emit(StreamsFailure(message: e.toString()));
      }
    } finally {
      _fetching = false;
    }
  }

  Future<List<LiveStream>> _fetch() =>
      _liveOnly ? _repository.fetchLiveStreams() : _repository.fetchAllStreams();

  @override
  Future<void> close() {
    // Without this the timer outlives the screen and keeps calling the API
    // through a bloc that can no longer emit.
    _poll?.cancel();
    return super.close();
  }
}
