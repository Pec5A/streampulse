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

class StreamsBloc extends Bloc<StreamsEvent, StreamsState> {
  StreamsBloc({required StreamRepository repository})
      : _repository = repository,
        super(const StreamsInitial()) {
    on<StreamsRequested>(_onRequested);
  }

  final StreamRepository _repository;

  Future<void> _onRequested(StreamsRequested event, Emitter<StreamsState> emit) async {
    emit(const StreamsLoading());
    try {
      final streams = event.liveOnly
          ? await _repository.fetchLiveStreams()
          : await _repository.fetchAllStreams();
      emit(StreamsLoaded(streams: streams, liveOnly: event.liveOnly));
    } on Object catch (e) {
      // A failure must be visible: showing an empty list would read as
      // "nobody is broadcasting" when the API is actually down.
      emit(StreamsFailure(message: e.toString()));
    }
  }
}
