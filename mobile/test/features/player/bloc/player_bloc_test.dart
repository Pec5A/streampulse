import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:streampulse/features/player/audio/audio_engine.dart';
import 'package:streampulse/features/player/bloc/player_bloc.dart';
import 'package:streampulse/features/player/models/live_stream.dart';

import '../fake_audio_engine.dart';

const liveStream = LiveStream(
  id: 's1',
  title: 'Jazz de nuit',
  description: 'session live',
  broadcasterId: 'u1',
  broadcasterUsername: 'kaysz',
  status: 'live',
  listenerCount: 3,
);

const recordedStream = LiveStream(
  id: 's2',
  title: 'Rediffusion',
  description: '',
  broadcasterId: 'u1',
  broadcasterUsername: 'kaysz',
  status: 'offline',
  listenerCount: 0,
);

void main() {
  late FakeAudioEngine engine;

  setUp(() => engine = FakeAudioEngine());

  PlayerBloc build() => PlayerBloc(engine: engine);

  group('selecting a stream', () {
    blocTest<PlayerBloc, PlayerStateData>(
      'loads the source and starts playing',
      build: build,
      act: (bloc) => bloc.add(const PlayerStreamSelected(
        stream: liveStream,
        url: 'http://api/api/v1/streams/s1/listen',
      )),
      verify: (_) {
        expect(engine.calls, [
          'setSource(http://api/api/v1/streams/s1/listen, isLive: true)',
          'play',
        ]);
      },
      expect: () => [
        isA<PlayerStateData>()
            .having((s) => s.status, 'status', PlaybackStatus.loading)
            .having((s) => s.stream?.id, 'stream', 's1'),
      ],
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'surfaces an engine failure instead of hanging on "loading"',
      build: () {
        engine.throwOnSetSource = Exception('connection refused');
        return build();
      },
      act: (bloc) => bloc.add(const PlayerStreamSelected(stream: liveStream, url: 'http://api/dead')),
      expect: () => [
        isA<PlayerStateData>().having((s) => s.status, 'status', PlaybackStatus.loading),
        isA<PlayerStateData>()
            .having((s) => s.status, 'status', PlaybackStatus.idle)
            .having((s) => s.errorMessage, 'errorMessage', contains('connection refused')),
      ],
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'clears the previous error and position when a new stream is selected',
      build: build,
      seed: () => const PlayerStateData(
        stream: recordedStream,
        position: Duration(seconds: 42),
        duration: Duration(minutes: 5),
        errorMessage: 'boom',
      ),
      act: (bloc) => bloc.add(const PlayerStreamSelected(stream: liveStream, url: 'http://api/s1')),
      expect: () => [
        isA<PlayerStateData>()
            .having((s) => s.position, 'position', Duration.zero)
            .having((s) => s.duration, 'duration', isNull)
            .having((s) => s.errorMessage, 'errorMessage', isNull),
      ],
    );
  });

  group('transport controls', () {
    blocTest<PlayerBloc, PlayerStateData>(
      'ignores play/pause when nothing is loaded',
      build: build,
      act: (bloc) => bloc
        ..add(const PlayerPlayRequested())
        ..add(const PlayerPauseRequested())
        ..add(const PlayerPlayPauseToggled()),
      expect: () => <PlayerStateData>[],
      verify: (_) => expect(engine.calls, isEmpty),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'toggle pauses while playing',
      build: build,
      seed: () => const PlayerStateData(stream: liveStream, status: PlaybackStatus.playing),
      act: (bloc) => bloc.add(const PlayerPlayPauseToggled()),
      verify: (_) => expect(engine.calls, ['pause']),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'toggle plays while paused',
      build: build,
      seed: () => const PlayerStateData(stream: liveStream, status: PlaybackStatus.paused),
      act: (bloc) => bloc.add(const PlayerPlayPauseToggled()),
      verify: (_) => expect(engine.calls, ['play']),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'stop resets to a clean state',
      build: build,
      seed: () => const PlayerStateData(
        stream: liveStream,
        status: PlaybackStatus.playing,
        position: Duration(seconds: 30),
        volume: 0.4,
      ),
      act: (bloc) => bloc.add(const PlayerStopRequested()),
      expect: () => [const PlayerStateData()],
      verify: (_) => expect(engine.calls, ['stop']),
    );
  });

  group('volume', () {
    blocTest<PlayerBloc, PlayerStateData>(
      'applies the new volume to the engine and the state',
      build: build,
      act: (bloc) => bloc.add(const PlayerVolumeChanged(0.25)),
      expect: () => [isA<PlayerStateData>().having((s) => s.volume, 'volume', 0.25)],
      verify: (_) => expect(engine.volume, 0.25),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'clamps out-of-range values',
      build: build,
      act: (bloc) => bloc
        ..add(const PlayerVolumeChanged(3))
        ..add(const PlayerVolumeChanged(-1)),
      expect: () => [
        isA<PlayerStateData>().having((s) => s.volume, 'volume', 1.0),
        isA<PlayerStateData>().having((s) => s.volume, 'volume', 0.0),
      ],
    );
  });

  group('seeking', () {
    blocTest<PlayerBloc, PlayerStateData>(
      'is refused on a live stream, which has no duration',
      build: build,
      seed: () => const PlayerStateData(stream: liveStream, status: PlaybackStatus.playing),
      act: (bloc) => bloc.add(const PlayerSeekRequested(Duration(seconds: 10))),
      expect: () => <PlayerStateData>[],
      verify: (_) => expect(engine.calls, isEmpty),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'moves the position on a stream with a known duration',
      build: build,
      seed: () => const PlayerStateData(
        stream: recordedStream,
        status: PlaybackStatus.playing,
        duration: Duration(minutes: 5),
      ),
      act: (bloc) => bloc.add(const PlayerSeekRequested(Duration(minutes: 2))),
      expect: () => [
        isA<PlayerStateData>().having((s) => s.position, 'position', const Duration(minutes: 2)),
      ],
      verify: (_) => expect(engine.calls, ['seek(0:02:00.000000)']),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'clamps a seek past the end back to the duration',
      build: build,
      seed: () => const PlayerStateData(
        stream: recordedStream,
        duration: Duration(minutes: 5),
      ),
      act: (bloc) => bloc.add(const PlayerSeekRequested(Duration(hours: 1))),
      expect: () => [
        isA<PlayerStateData>().having((s) => s.position, 'position', const Duration(minutes: 5)),
      ],
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'clamps a negative seek back to zero',
      build: build,
      seed: () => const PlayerStateData(
        stream: recordedStream,
        position: Duration(minutes: 1),
        duration: Duration(minutes: 5),
      ),
      act: (bloc) => bloc.add(const PlayerSeekRequested(Duration(seconds: -30))),
      expect: () => [
        isA<PlayerStateData>().having((s) => s.position, 'position', Duration.zero),
      ],
    );
  });

  group('engine-driven updates', () {
    blocTest<PlayerBloc, PlayerStateData>(
      'follows the engine status',
      build: build,
      act: (bloc) async {
        engine.statusController.add(PlaybackStatus.playing);
        await Future<void>.delayed(Duration.zero);
        engine.statusController.add(PlaybackStatus.paused);
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [
        isA<PlayerStateData>().having((s) => s.status, 'status', PlaybackStatus.playing),
        isA<PlayerStateData>().having((s) => s.status, 'status', PlaybackStatus.paused),
      ],
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'a null duration marks the stream unseekable',
      build: build,
      seed: () => const PlayerStateData(stream: liveStream, duration: Duration(minutes: 3)),
      act: (bloc) async {
        engine.durationController.add(null);
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [
        isA<PlayerStateData>()
            .having((s) => s.duration, 'duration', isNull)
            .having((s) => s.canSeek, 'canSeek', isFalse),
      ],
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'tracks the playback position',
      build: build,
      act: (bloc) async {
        engine.positionController.add(const Duration(seconds: 7));
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [
        isA<PlayerStateData>().having((s) => s.position, 'position', const Duration(seconds: 7)),
      ],
    );
  });

  group('OS interruptions', () {
    blocTest<PlayerBloc, PlayerStateData>(
      'an incoming call pauses playback and flags the state',
      build: build,
      seed: () => const PlayerStateData(stream: liveStream, status: PlaybackStatus.playing),
      act: (bloc) async {
        engine.interruptionController.add(const InterruptionEvent(InterruptionKind.begin));
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [isA<PlayerStateData>().having((s) => s.interrupted, 'interrupted', isTrue)],
      verify: (_) => expect(engine.calls, ['pause']),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'playback resumes after a call the OS says we may resume',
      build: build,
      seed: () => const PlayerStateData(
        stream: liveStream,
        status: PlaybackStatus.paused,
        interrupted: true,
      ),
      act: (bloc) async {
        engine.interruptionController.add(
          const InterruptionEvent(InterruptionKind.end, shouldResume: true),
        );
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [isA<PlayerStateData>().having((s) => s.interrupted, 'interrupted', isFalse)],
      verify: (_) => expect(engine.calls, ['play']),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'playback stays paused when the OS does not want us back',
      build: build,
      seed: () => const PlayerStateData(
        stream: liveStream,
        status: PlaybackStatus.paused,
        interrupted: true,
      ),
      act: (bloc) async {
        engine.interruptionController.add(const InterruptionEvent(InterruptionKind.end));
        await Future<void>.delayed(Duration.zero);
      },
      expect: () => [isA<PlayerStateData>().having((s) => s.interrupted, 'interrupted', isFalse)],
      verify: (_) => expect(engine.calls, isEmpty),
    );

    blocTest<PlayerBloc, PlayerStateData>(
      'does not resume when no stream is loaded',
      build: build,
      act: (bloc) async {
        engine.interruptionController.add(
          const InterruptionEvent(InterruptionKind.end, shouldResume: true),
        );
        await Future<void>.delayed(Duration.zero);
      },
      verify: (_) => expect(engine.calls, isEmpty),
    );
  });

  test('closing the bloc disposes the engine', () async {
    final bloc = build();
    await bloc.close();
    expect(engine.disposed, isTrue);
  });

  group('PlayerStateData', () {
    test('canSeek requires a non-zero duration', () {
      expect(const PlayerStateData().canSeek, isFalse);
      expect(const PlayerStateData(duration: Duration.zero).canSeek, isFalse);
      expect(const PlayerStateData(duration: Duration(minutes: 1)).canSeek, isTrue);
    });

    test('copyWith can explicitly clear the error and duration', () {
      const state = PlayerStateData(errorMessage: 'boom', duration: Duration(minutes: 1));
      final cleared = state.copyWith(clearError: true, clearDuration: true);
      expect(cleared.errorMessage, isNull);
      expect(cleared.duration, isNull);
    });

    test('copyWith keeps untouched fields', () {
      const state = PlayerStateData(stream: liveStream, volume: 0.3, position: Duration(seconds: 5));
      final next = state.copyWith(status: PlaybackStatus.playing);
      expect(next.volume, 0.3);
      expect(next.position, const Duration(seconds: 5));
      expect(next.stream, liveStream);
    });
  });
}
