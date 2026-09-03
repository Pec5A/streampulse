import 'package:flutter_test/flutter_test.dart';
import 'package:just_audio/just_audio.dart';
import 'package:mocktail/mocktail.dart';
import 'package:streampulse/features/player/audio/just_audio_engine.dart';

class MockAudioPlayer extends Mock implements AudioPlayer {}

const liveUrl = 'http://api.test/api/v1/streams/s1/listen';
const trackUrl = 'http://api.test/api/v1/tracks/t1/audio';

void main() {
  late MockAudioPlayer player;
  late JustAudioEngine engine;

  /// Matches a source being opened either way, so "never reopened" cannot pass
  /// just because the call carried a named argument.
  void verifyNeverOpened() {
    verifyNever(() => player.setUrl(any()));
    verifyNever(() => player.setUrl(any(), preload: any(named: 'preload')));
  }

  setUp(() {
    player = MockAudioPlayer();
    engine = JustAudioEngine(player: player);

    when(() => player.setUrl(any())).thenAnswer((_) async => null);
    when(() => player.setUrl(any(), preload: any(named: 'preload')))
        .thenAnswer((_) async => null);
    when(player.play).thenAnswer((_) async {});
    when(player.pause).thenAnswer((_) async {});
    when(player.stop).thenAnswer((_) async {});
  });

  group('a live stream', () {
    test('opens without preloading', () async {
      // Preloading waits for bytes that only exist in real time: the broadcast
      // arrives at the speed it plays, so the call returns late or never.
      await engine.setSource(liveUrl, isLive: true);

      verify(() => player.setUrl(liveUrl, preload: false)).called(1);
    });

    test('pause releases the connection instead of stalling it', () async {
      // Holding the socket open without reading it is what got the listener
      // evicted: the hub's per-listener buffer fills, and after enough
      // consecutive drops it drops the subscriber to stay bounded.
      await engine.setSource(liveUrl, isLive: true);
      await engine.pause();

      verify(player.stop).called(1);
      verifyNever(player.pause);
    });

    test('play after a pause reconnects before playing', () async {
      await engine.setSource(liveUrl, isLive: true);
      clearInteractions(player);

      await engine.pause();
      await engine.play();

      verifyInOrder([
        player.stop,
        () => player.setUrl(liveUrl, preload: false),
        player.play,
      ]);
    });

    test('reconnects on every pause, not just the first', () async {
      // The first resume used to work and later ones did not; every cycle must
      // reopen the same way.
      await engine.setSource(liveUrl, isLive: true);
      clearInteractions(player);

      for (var i = 0; i < 3; i++) {
        await engine.pause();
        await engine.play();
      }

      verify(() => player.setUrl(liveUrl, preload: false)).called(3);
    });

    test('play without a preceding pause does not reconnect', () async {
      // Reopening on every play would drop and rebuild a healthy connection.
      await engine.setSource(liveUrl, isLive: true);
      clearInteractions(player);

      await engine.play();

      verifyNeverOpened();
      verify(player.play).called(1);
    });

    test('stop also marks the source for reconnection', () async {
      await engine.setSource(liveUrl, isLive: true);
      clearInteractions(player);

      await engine.stop();
      await engine.play();

      verify(() => player.setUrl(liveUrl, preload: false)).called(1);
    });
  });

  group('a finite track', () {
    test('preloads, because there is a whole file to load', () async {
      await engine.setSource(trackUrl, isLive: false);

      verify(() => player.setUrl(trackUrl)).called(1);
      verifyNever(() => player.setUrl(any(), preload: any(named: 'preload')));
    });

    test('pause really pauses, so playback resumes where it stopped', () async {
      await engine.setSource(trackUrl, isLive: false);
      clearInteractions(player);

      await engine.pause();
      await engine.play();

      verify(player.pause).called(1);
      verifyNever(player.stop);
      // No reconnection: the position is worth keeping for a file.
      verifyNeverOpened();
    });

    test('stop does not turn the next play into a reconnection', () async {
      await engine.setSource(trackUrl, isLive: false);
      clearInteractions(player);

      await engine.stop();
      await engine.play();

      verifyNeverOpened();
    });
  });

  test('a new source clears a pending reconnection', () async {
    await engine.setSource(liveUrl, isLive: true);
    await engine.pause();

    await engine.setSource(trackUrl, isLive: false);
    clearInteractions(player);
    await engine.play();

    // setSource already opened the new URL; play must not reopen it.
    verifyNeverOpened();
    verify(player.play).called(1);
  });
}
