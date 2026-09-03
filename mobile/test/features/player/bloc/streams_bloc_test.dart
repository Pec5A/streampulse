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

const other = LiveStream(
  id: 's2',
  title: 'Set techno',
  description: '',
  broadcasterId: 'u2',
  broadcasterUsername: 'yassir',
  status: 'live',
  listenerCount: 0,
);

void main() {
  late MockStreamRepository repository;

  setUp(() => repository = MockStreamRepository());

  blocTest<StreamsBloc, StreamsState>(
    'emits [Loading, Loaded] with the live streams',
    setUp: () => when(repository.fetchLiveStreams).thenAnswer((_) async => [stream]),
    build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
    act: (bloc) => bloc.add(const StreamsRequested()),
    expect: () => [
      const StreamsLoading(),
      const StreamsLoaded(streams: [stream], liveOnly: true),
    ],
  );

  blocTest<StreamsBloc, StreamsState>(
    'queries the full catalogue when liveOnly is false',
    setUp: () => when(repository.fetchAllStreams).thenAnswer((_) async => [stream]),
    build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
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
    build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
    act: (bloc) => bloc.add(const StreamsRequested()),
    expect: () => [
      const StreamsLoading(),
      isA<StreamsFailure>().having((s) => s.message, 'message', contains('stream operation failed')),
    ],
  );

  blocTest<StreamsBloc, StreamsState>(
    'emits Loaded with an empty list when nobody is on air',
    setUp: () => when(repository.fetchLiveStreams).thenAnswer((_) async => []),
    build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
    act: (bloc) => bloc.add(const StreamsRequested()),
    expect: () => [
      const StreamsLoading(),
      const StreamsLoaded(streams: [], liveOnly: true),
    ],
  );

  group('silent refresh', () {
    blocTest<StreamsBloc, StreamsState>(
      'updates the list in place, without going through Loading',
      // Polling with StreamsRequested would blank the list into a spinner
      // every five seconds.
      setUp: () {
        var call = 0;
        when(repository.fetchLiveStreams).thenAnswer((_) async => call++ == 0 ? [stream] : [stream, other]);
      },
      build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
      act: (bloc) async {
        bloc.add(const StreamsRequested());
        await bloc.stream.firstWhere((s) => s is StreamsLoaded);
        bloc.add(const StreamsRefreshRequested());
      },
      expect: () => [
        const StreamsLoading(),
        const StreamsLoaded(streams: [stream], liveOnly: true),
        const StreamsLoaded(streams: [stream, other], liveOnly: true),
      ],
    );

    blocTest<StreamsBloc, StreamsState>(
      'keeps the last good list when a refresh fails',
      // A five-second poll that flickers to an error page on one bad response
      // is worse than a list a few seconds stale.
      setUp: () {
        var call = 0;
        when(repository.fetchLiveStreams).thenAnswer((_) async {
          if (call++ == 0) return [stream];
          throw ApiException(503, 'service unavailable');
        });
      },
      build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
      act: (bloc) async {
        bloc.add(const StreamsRequested());
        await bloc.stream.firstWhere((s) => s is StreamsLoaded);
        bloc.add(const StreamsRefreshRequested());
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [
        const StreamsLoading(),
        const StreamsLoaded(streams: [stream], liveOnly: true),
      ],
    );

    blocTest<StreamsBloc, StreamsState>(
      'reports the failure when there is no list worth preserving',
      setUp: () => when(repository.fetchLiveStreams).thenThrow(ApiException(503, 'service unavailable')),
      build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
      act: (bloc) => bloc.add(const StreamsRefreshRequested()),
      expect: () => [
        isA<StreamsFailure>().having((s) => s.message, 'message', contains('service unavailable')),
      ],
    );

    blocTest<StreamsBloc, StreamsState>(
      'recovers to Loaded once the API answers again',
      setUp: () {
        var call = 0;
        when(repository.fetchLiveStreams).thenAnswer((_) async {
          if (call++ == 0) throw ApiException(503, 'down');
          return [stream];
        });
      },
      build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
      act: (bloc) async {
        bloc.add(const StreamsRefreshRequested());
        await bloc.stream.firstWhere((s) => s is StreamsFailure);
        bloc.add(const StreamsRefreshRequested());
      },
      expect: () => [
        isA<StreamsFailure>(),
        const StreamsLoaded(streams: [stream], liveOnly: true),
      ],
    );

    blocTest<StreamsBloc, StreamsState>(
      'refreshes the mode the user is looking at',
      setUp: () {
        when(repository.fetchAllStreams).thenAnswer((_) async => [stream]);
        when(repository.fetchLiveStreams).thenAnswer((_) async => []);
      },
      build: () => StreamsBloc(repository: repository, pollInterval: Duration.zero),
      act: (bloc) async {
        bloc.add(const StreamsRequested(liveOnly: false));
        await bloc.stream.firstWhere((s) => s is StreamsLoaded);
        bloc.add(const StreamsRefreshRequested());
        await Future<void>.delayed(Duration.zero);
      },
      // A poll must not silently switch the list out from under the user.
      verify: (_) => verifyNever(repository.fetchLiveStreams),
    );
  });

  group('polling', () {
    test('re-reads the catalogue on the interval', () async {
      when(repository.fetchLiveStreams).thenAnswer((_) async => [stream]);
      final bloc = StreamsBloc(
        repository: repository,
        pollInterval: const Duration(milliseconds: 20),
      );

      await Future<void>.delayed(const Duration(milliseconds: 110));
      await bloc.close();

      // Nothing tells the app a broadcast started; without the timer a stream
      // that goes live elsewhere stays invisible.
      verify(repository.fetchLiveStreams).called(greaterThan(2));
    });

    test('stops calling the API once closed', () async {
      when(repository.fetchLiveStreams).thenAnswer((_) async => [stream]);
      final bloc = StreamsBloc(
        repository: repository,
        pollInterval: const Duration(milliseconds: 20),
      );

      await Future<void>.delayed(const Duration(milliseconds: 50));
      await bloc.close();
      // Consumes every call recorded so far, so anything counted after this
      // point can only have happened post-close.
      verify(repository.fetchLiveStreams).called(greaterThan(0));

      // A timer that outlives the screen keeps hitting the API through a bloc
      // that can no longer emit.
      await Future<void>.delayed(const Duration(milliseconds: 80));
      verifyNever(repository.fetchLiveStreams);
    });

    test('does not stack requests when the API is slower than the interval', () async {
      when(repository.fetchLiveStreams).thenAnswer(
        (_) async => Future<List<LiveStream>>.delayed(const Duration(milliseconds: 100), () => [stream]),
      );
      final bloc = StreamsBloc(
        repository: repository,
        pollInterval: const Duration(milliseconds: 10),
      );

      await Future<void>.delayed(const Duration(milliseconds: 120));
      await bloc.close();

      // Ten ticks fired, but each waits for the one in flight to land.
      verify(repository.fetchLiveStreams).called(lessThan(4));
    });
  });
}
