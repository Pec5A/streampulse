import 'dart:async';

import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mocktail/mocktail.dart';
import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/features/broadcaster/bloc/broadcaster_bloc.dart';
import 'package:streampulse/features/broadcaster/broadcast/audio_file_picker.dart';
import 'package:streampulse/features/broadcaster/broadcast/broadcast_transport.dart';
import 'package:streampulse/features/broadcaster/models/track.dart';
import 'package:streampulse/features/broadcaster/repositories/broadcaster_repository.dart';
import 'package:streampulse/features/player/models/live_stream.dart';

class MockBroadcasterRepository extends Mock implements BroadcasterRepository {}

/// Records what the bloc asked of the transport, so a test can assert that a
/// stop really reached it and not just the state.
class FakeBroadcastTransport implements BroadcastTransport {
  final calls = <String>[];
  bool _running = false;
  Object? throwOnStart;

  @override
  bool get isBroadcasting => _running;

  @override
  Future<void> start({
    required String publishUrl,
    required String token,
    required Stream<List<int>> source,
  }) async {
    calls.add('start($publishUrl)');
    // Strict like HttpBroadcastTransport: a fake that accepts what the real
    // one refuses lets a broken call path pass its tests.
    if (_running) throw StateError('a broadcast is already running');
    if (throwOnStart != null) throw throwOnStart!;
    _running = true;
  }

  /// Puts the fake in the state a real transport reaches after [start], for
  /// tests that seed a bloc already on air instead of going live first.
  void simulateRunning() => _running = true;

  @override
  Future<void> switchSource(Stream<List<int>> source) async {
    calls.add('switchSource');
    if (!_running) throw StateError('no broadcast to switch');
  }

  @override
  Future<void> stop() async {
    calls.add('stop');
    _running = false;
  }
}

class FakePicker implements AudioFilePicker {
  FakePicker(this.result);
  final PickedAudio? result;
  Object? throwOnPick;
  int calls = 0;

  @override
  Future<PickedAudio?> pick() async {
    calls++;
    if (throwOnPick != null) throw throwOnPick!;
    return result;
  }
}

const track = Track(
  id: 't1',
  title: 'Nocturne',
  artist: 'KaysZ',
  contentType: 'audio/mpeg',
  sizeBytes: 4096,
  uploaderId: 'u1',
  uploaderUsername: 'kaysz',
  audioUrl: '/api/v1/tracks/t1/audio',
);

const otherTrack = Track(
  id: 't2',
  title: 'Aube',
  artist: 'KaysZ',
  contentType: 'audio/mpeg',
  sizeBytes: 8192,
  uploaderId: 'u1',
  uploaderUsername: 'kaysz',
  audioUrl: '/api/v1/tracks/t2/audio',
);

const stream = LiveStream(
  id: 's1',
  title: 'Jazz de nuit',
  description: '',
  broadcasterId: 'u1',
  broadcasterUsername: 'kaysz',
  status: 'offline',
  listenerCount: 0,
);

const picked = PickedAudio(filename: 'nocturne.mp3', bytes: [1, 2, 3], contentType: 'audio/mpeg');

void main() {
  late MockBroadcasterRepository repository;
  late FakeBroadcastTransport transport;
  late FakePicker picker;

  setUpAll(() {
    // mocktail needs a concrete instance for every type used with any().
    registerFallbackValue(picked);
    registerFallbackValue(track);
  });

  setUp(() {
    repository = MockBroadcasterRepository();
    transport = FakeBroadcastTransport();
    picker = FakePicker(picked);

    when(() => repository.publishUrl(any())).thenReturn('http://api.test/api/v1/streams/s1/publish');
    when(() => repository.trackAudioUrl(any())).thenReturn('http://api.test/api/v1/tracks/t1/audio');
  });

  // audioClient stands in for the GET that fetches the track being broadcast.
  BroadcasterBloc build({MockClient? audioClient}) => BroadcasterBloc(
        repository: repository,
        picker: picker,
        transport: transport,
        token: 'jwt-token',
        audioClient: audioClient ??
            MockClient((_) async => http.Response.bytes(List<int>.filled(64, 7), 200)),
      );

  group('loading my tracks', () {
    blocTest<BroadcasterBloc, BroadcasterState>(
      'fills the library',
      setUp: () => when(() => repository.fetchMyTracks(any())).thenAnswer((_) async => [track]),
      build: build,
      act: (bloc) => bloc.add(const BroadcasterTracksRequested()),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.loading, 'loading', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.loading, 'loading', isFalse)
            .having((s) => s.tracks, 'tracks', [track]),
      ],
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'shows the failure instead of an empty library',
      setUp: () => when(() => repository.fetchMyTracks(any())).thenThrow(ApiException(500, 'boom')),
      build: build,
      act: (bloc) => bloc.add(const BroadcasterTracksRequested()),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.loading, 'loading', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.loading, 'loading', isFalse)
            .having((s) => s.errorMessage, 'errorMessage', contains('boom')),
      ],
    );
  });

  group('picking and uploading', () {
    blocTest<BroadcasterBloc, BroadcasterState>(
      'a cancelled picker leaves the state untouched',
      build: () {
        picker = FakePicker(null);
        return build();
      },
      act: (bloc) => bloc.add(const BroadcasterTrackPicked()),
      expect: () => <BroadcasterState>[],
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'a picked file becomes the pending upload',
      build: build,
      act: (bloc) => bloc.add(const BroadcasterTrackPicked()),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.pendingUpload, 'pendingUpload', picked),
      ],
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'refuses to upload with nothing picked',
      build: build,
      act: (bloc) => bloc.add(const BroadcasterUploadRequested(title: 'Nocturne')),
      expect: () => [
        isA<BroadcasterState>()
            .having((s) => s.errorMessage, 'errorMessage', contains('Choisis un fichier')),
      ],
      verify: (_) => verifyNever(() => repository.uploadTrack(
            token: any(named: 'token'),
            title: any(named: 'title'),
            artist: any(named: 'artist'),
            audio: any(named: 'audio'),
          )),
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'refuses to upload without a title',
      build: build,
      seed: () => const BroadcasterState(pendingUpload: picked),
      act: (bloc) => bloc.add(const BroadcasterUploadRequested(title: '   ')),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.errorMessage, 'errorMessage', contains('titre')),
      ],
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'uploads, prepends the track and clears the pending file',
      setUp: () => when(() => repository.uploadTrack(
            token: any(named: 'token'),
            title: any(named: 'title'),
            artist: any(named: 'artist'),
            audio: any(named: 'audio'),
          )).thenAnswer((_) async => track),
      build: build,
      seed: () => const BroadcasterState(pendingUpload: picked),
      act: (bloc) => bloc.add(const BroadcasterUploadRequested(title: '  Nocturne  ', artist: 'KaysZ')),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.uploading, 'uploading', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.uploading, 'uploading', isFalse)
            .having((s) => s.tracks, 'tracks', [track])
            .having((s) => s.pendingUpload, 'pendingUpload', isNull),
      ],
      verify: (_) => verify(() => repository.uploadTrack(
            token: 'jwt-token',
            title: 'Nocturne', // trimmed
            artist: 'KaysZ',
            audio: picked,
          )).called(1),
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'keeps the pending file when the upload is rejected, so it can be retried',
      setUp: () => when(() => repository.uploadTrack(
            token: any(named: 'token'),
            title: any(named: 'title'),
            artist: any(named: 'artist'),
            audio: any(named: 'audio'),
          )).thenThrow(ApiException(415, 'unsupported media type')),
      build: build,
      seed: () => const BroadcasterState(pendingUpload: picked),
      act: (bloc) => bloc.add(const BroadcasterUploadRequested(title: 'Nocturne')),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.uploading, 'uploading', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.uploading, 'uploading', isFalse)
            .having((s) => s.errorMessage, 'errorMessage', contains('unsupported media type'))
            .having((s) => s.pendingUpload, 'pendingUpload', picked),
      ],
    );
  });

  group('creating a stream', () {
    blocTest<BroadcasterBloc, BroadcasterState>(
      'stores the created stream',
      setUp: () => when(() => repository.createStream(
            token: any(named: 'token'),
            title: any(named: 'title'),
            description: any(named: 'description'),
          )).thenAnswer((_) async => stream),
      build: build,
      act: (bloc) => bloc.add(const BroadcasterStreamCreated(title: 'Jazz de nuit')),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.loading, 'loading', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.loading, 'loading', isFalse)
            .having((s) => s.stream?.id, 'stream', 's1'),
      ],
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'refuses an empty title without calling the API',
      build: build,
      act: (bloc) => bloc.add(const BroadcasterStreamCreated(title: '  ')),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.errorMessage, 'errorMessage', contains('titre')),
      ],
      verify: (_) => verifyNever(() => repository.createStream(
            token: any(named: 'token'),
            title: any(named: 'title'),
            description: any(named: 'description'),
          )),
    );
  });

  group('going live', () {
    blocTest<BroadcasterBloc, BroadcasterState>(
      'refuses without a stream',
      build: build,
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(track)),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.errorMessage, 'errorMessage', contains('Crée un direct')),
      ],
      // close() also calls stop(), so assert on starts, not the exact list.
      verify: (_) => expect(transport.calls.where((c) => c.startsWith('start')), isEmpty),
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'starts the transport and flags the state live',
      build: build,
      seed: () => const BroadcasterState(stream: stream, tracks: [track]),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(track)),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.starting, 'starting', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.starting, 'starting', isFalse)
            .having((s) => s.isLive, 'isLive', isTrue)
            .having((s) => s.broadcastingTrack?.id, 'broadcastingTrack', 't1'),
      ],
      verify: (_) => expect(transport.calls.first, 'start(http://api.test/api/v1/streams/s1/publish)'),
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'restarts the same track rather than refusing, when already live',
      setUp: () => transport.simulateRunning(),
      // Being on air no longer blocks a new broadcast: it is how the track is
      // changed. Asking for the one already playing restarts it, on air.
      build: build,
      seed: () => const BroadcasterState(stream: stream, tracks: [track], isLive: true),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(track)),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.starting, 'starting', isTrue),
        isA<BroadcasterState>().having((s) => s.isLive, 'isLive', isTrue),
      ],
      verify: (_) => expect(transport.calls, ['switchSource', 'stop']),
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'stops the transport if the audio source cannot be fetched',
      // Otherwise the stream would be marked live on the server with nothing
      // ever published to it — a dead entry in the listeners' catalogue.
      build: () => build(audioClient: MockClient((_) async => http.Response('not found', 404))),
      seed: () => const BroadcasterState(stream: stream, tracks: [track]),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(track)),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.starting, 'starting', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.starting, 'starting', isFalse)
            .having((s) => s.isLive, 'isLive', isFalse)
            .having((s) => s.errorMessage, 'errorMessage', contains('404')),
      ],
      verify: (_) => expect(transport.calls, contains('stop')),
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'stops the transport if starting it throws',
      build: () {
        transport.throwOnStart = StateError('socket closed');
        return build();
      },
      seed: () => const BroadcasterState(stream: stream, tracks: [track]),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(track)),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.starting, 'starting', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.isLive, 'isLive', isFalse)
            .having((s) => s.errorMessage, 'errorMessage', contains('socket closed')),
      ],
      verify: (_) => expect(transport.calls.last, 'stop'),
    );
  });

  group('stopping', () {
    blocTest<BroadcasterBloc, BroadcasterState>(
      'stops the transport and clears what was on air',
      build: build,
      seed: () => const BroadcasterState(
        stream: stream,
        tracks: [track],
        isLive: true,
        broadcastingTrack: track,
      ),
      act: (bloc) => bloc.add(const BroadcasterStopRequested()),
      expect: () => [
        isA<BroadcasterState>()
            .having((s) => s.isLive, 'isLive', isFalse)
            .having((s) => s.broadcastingTrack, 'broadcastingTrack', isNull),
      ],
      verify: (_) => expect(transport.calls.first, 'stop'),
    );
  });

  group('deleting a track', () {
    blocTest<BroadcasterBloc, BroadcasterState>(
      'removes it from the library',
      setUp: () => when(() => repository.deleteTrack(
            token: any(named: 'token'),
            trackId: any(named: 'trackId'),
          )).thenAnswer((_) async {}),
      build: build,
      seed: () => const BroadcasterState(tracks: [track]),
      act: (bloc) => bloc.add(const BroadcasterTrackDeleted('t1')),
      expect: () => [isA<BroadcasterState>().having((s) => s.tracks, 'tracks', isEmpty)],
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'keeps it in the library when the API refuses',
      setUp: () => when(() => repository.deleteTrack(
            token: any(named: 'token'),
            trackId: any(named: 'trackId'),
          )).thenThrow(ApiException(403, 'not your track')),
      build: build,
      seed: () => const BroadcasterState(tracks: [track]),
      act: (bloc) => bloc.add(const BroadcasterTrackDeleted('t1')),
      expect: () => [
        isA<BroadcasterState>()
            .having((s) => s.tracks, 'tracks', [track])
            .having((s) => s.errorMessage, 'errorMessage', contains('not your track')),
      ],
    );
  });

  test('closing the bloc stops any running broadcast', () async {
    when(() => repository.fetchMyTracks(any())).thenAnswer((_) async => []);
    final bloc = build();

    await bloc.close();

    expect(transport.calls, contains('stop'));
  });

  group('BroadcasterState', () {
    test('canGoLive needs a stream and a track, and no start in flight', () {
      expect(const BroadcasterState().canGoLive, isFalse);
      expect(const BroadcasterState(stream: stream).canGoLive, isFalse);
      expect(const BroadcasterState(stream: stream, tracks: [track]).canGoLive, isTrue);
      // True while live on purpose: a station changes record without going
      // off the air first.
      expect(const BroadcasterState(stream: stream, tracks: [track], isLive: true).canGoLive, isTrue);
      expect(const BroadcasterState(stream: stream, tracks: [track], starting: true).canGoLive, isFalse);
    });

    test('copyWith can explicitly clear the nullable fields', () {
      const state = BroadcasterState(
        pendingUpload: picked,
        broadcastingTrack: track,
        errorMessage: 'boom',
      );
      final cleared = state.copyWith(
        clearPendingUpload: true,
        clearBroadcastingTrack: true,
        clearError: true,
      );
      expect(cleared.pendingUpload, isNull);
      expect(cleared.broadcastingTrack, isNull);
      expect(cleared.errorMessage, isNull);
    });
  });

  group('changing track on air', () {
    blocTest<BroadcasterBloc, BroadcasterState>(
      'swaps the source without closing the connection',
      setUp: () => transport.simulateRunning(),
      // Stopping and restarting would flip the stream offline and end every
      // listener's response: the audience would be dropped by a change of
      // record. The open request is reused instead.
      build: build,
      seed: () => const BroadcasterState(
        stream: stream,
        tracks: [track, otherTrack],
        isLive: true,
        broadcastingTrack: track,
      ),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(otherTrack)),
      // The full sequence, not a prefix: a prefix would also pass if the
      // switch had thrown and the catch had stopped the broadcast. The
      // trailing stop is close()'s teardown.
      verify: (_) => expect(transport.calls, ['switchSource', 'stop']),
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'stays on air, with the new track',
      setUp: () => transport.simulateRunning(),
      build: build,
      seed: () => const BroadcasterState(
        stream: stream,
        tracks: [track, otherTrack],
        isLive: true,
        broadcastingTrack: track,
      ),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(otherTrack)),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.starting, 'starting', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.isLive, 'isLive', isTrue)
            .having((s) => s.broadcastingTrack, 'broadcastingTrack', otherTrack),
      ],
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'a track that cannot be fetched does not take the station off air',
      // The transport was never asked for anything: the running broadcast is
      // intact, and ending it over a failed download would be a worse outcome
      // than the error itself.
      setUp: () => transport.simulateRunning(),
      build: () => build(audioClient: MockClient((_) async => http.Response('nope', 404))),
      seed: () => const BroadcasterState(
        stream: stream,
        tracks: [track, otherTrack],
        isLive: true,
        broadcastingTrack: track,
      ),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(otherTrack)),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.starting, 'starting', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.isLive, 'isLive', isTrue)
            .having((s) => s.broadcastingTrack, 'broadcastingTrack', track)
            .having((s) => s.errorMessage, 'errorMessage', contains('404')),
      ],
      verify: (_) => expect(transport.calls, ['stop']), // close() only
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'a failed first broadcast stops claiming an antenna it never held',
      build: () => build(audioClient: MockClient((_) async => http.Response('nope', 404))),
      seed: () => const BroadcasterState(stream: stream, tracks: [track]),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(track)),
      expect: () => [
        isA<BroadcasterState>().having((s) => s.starting, 'starting', isTrue),
        isA<BroadcasterState>()
            .having((s) => s.isLive, 'isLive', isFalse)
            .having((s) => s.broadcastingTrack, 'broadcastingTrack', isNull),
      ],
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'a second tap in the same frame is ignored',
      // Bloc runs handlers concurrently by default: without the guard the
      // second run fails and its catch cuts the broadcast the first started.
      build: build,
      seed: () => const BroadcasterState(stream: stream, tracks: [track, otherTrack]),
      act: (bloc) {
        bloc.add(const BroadcasterGoLiveRequested(track));
        bloc.add(const BroadcasterGoLiveRequested(otherTrack));
      },
      verify: (_) {
        expect(transport.calls.where((c) => c.startsWith('start')), hasLength(1));
        expect(transport.calls, isNot(contains('switchSource')));
      },
    );

    blocTest<BroadcasterBloc, BroadcasterState>(
      'a first broadcast still starts without a stop',
      build: build,
      seed: () => const BroadcasterState(stream: stream, tracks: [track]),
      act: (bloc) => bloc.add(const BroadcasterGoLiveRequested(track)),
      verify: (_) => expect(transport.calls, ['start(http://api.test/api/v1/streams/s1/publish)', 'stop']),
    );
  });
}
