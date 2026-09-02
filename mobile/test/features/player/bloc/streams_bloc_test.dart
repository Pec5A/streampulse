import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/features/player/bloc/streams_bloc.dart';
import 'package:streampulse/features/player/models/live_stream.dart';
import 'package:streampulse/features/player/repositories/stream_repository.dart';

class MockStreamRepository extends Mock implements StreamRepository {}

const stream = LiveStream(
  id: 's1',
  title: 'Jazz de nuit',
  description: '',
  broadcasterId: 'u1',
  broadcasterUsername: 'kaysz',
  status: 'live',
  listenerCount: 2,
);

void main() {
  late MockStreamRepository repository;

  setUp(() => repository = MockStreamRepository());

  blocTest<StreamsBloc, StreamsState>(
    'emits [Loading, Loaded] with the live streams',
    setUp: () => when(repository.fetchLiveStreams).thenAnswer((_) async => [stream]),
    build: () => StreamsBloc(repository: repository),
    act: (bloc) => bloc.add(const StreamsRequested()),
    expect: () => [
      const StreamsLoading(),
      const StreamsLoaded(streams: [stream], liveOnly: true),
    ],
  );

  blocTest<StreamsBloc, StreamsState>(
    'queries the full catalogue when liveOnly is false',
    setUp: () => when(repository.fetchAllStreams).thenAnswer((_) async => [stream]),
    build: () => StreamsBloc(repository: repository),
    act: (bloc) => bloc.add(const StreamsRequested(liveOnly: false)),
    expect: () => [
      const StreamsLoading(),
      const StreamsLoaded(streams: [stream], liveOnly: false),
    ],
    verify: (_) => verifyNever(repository.fetchLiveStreams),
  );

  blocTest<StreamsBloc, StreamsState>(
    'emits Failure rather than an empty list when the API is down',
    // An empty list would read as "nobody is broadcasting" and hide the
    // outage from the user.
    setUp: () => when(repository.fetchLiveStreams).thenThrow(ApiException(500, 'stream operation failed')),
    build: () => StreamsBloc(repository: repository),
    act: (bloc) => bloc.add(const StreamsRequested()),
    expect: () => [
      const StreamsLoading(),
      isA<StreamsFailure>().having((s) => s.message, 'message', contains('stream operation failed')),
    ],
  );

  blocTest<StreamsBloc, StreamsState>(
    'emits Loaded with an empty list when nobody is on air',
    setUp: () => when(repository.fetchLiveStreams).thenAnswer((_) async => []),
    build: () => StreamsBloc(repository: repository),
    act: (bloc) => bloc.add(const StreamsRequested()),
    expect: () => [
      const StreamsLoading(),
      const StreamsLoaded(streams: [], liveOnly: true),
    ],
  );
}
