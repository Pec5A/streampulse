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

  setUp(() {
    player = MockAudioPlayer();
    engine = JustAudioEngine(player: player);

    when(() => player.setUrl(any())).thenAnswer((_) async => null);
    when(player.play).thenAnswer((_) async {});
    when(player.pause).thenAnswer((_) async {});
    when(player.stop).thenAnswer((_) async {});
  });

  group('a live stream', () {
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
        () => player.setUrl(liveUrl),
        player.play,
      ]);
    });

    test('reconnects on every pause, not just the first', () async {
      await engine.setSource(liveUrl, isLive: true);
      clearInteractions(player);

      for (var i = 0; i < 3; i++) {
        await engine.pause();
        await engine.play();
      }

      verify(() => player.setUrl(liveUrl)).called(3);
    });

    test('play without a preceding pause does not reconnect', () async {
      // Reopening on every play would drop and rebuild a healthy connection.
      await engine.setSource(liveUrl, isLive: true);
      clearInteractions(player);

      await engine.play();

      verifyNever(() => player.setUrl(any()));
      verify(player.play).called(1);
    });

    test('stop also marks the source for reconnection', () async {
      await engine.setSource(liveUrl, isLive: true);
      clearInteractions(player);

      await engine.stop();
      await engine.play();

      verify(() => player.setUrl(liveUrl)).called(1);
    });
  });

  group('a finite track', () {
    test('pause really pauses, so playback resumes where it stopped', () async {
      await engine.setSource(trackUrl, isLive: false);
      clearInteractions(player);

      await engine.pause();
      await engine.play();

      verify(player.pause).called(1);
      verifyNever(player.stop);
      // No reconnection: the position is worth keeping for a file.
      verifyNever(() => player.setUrl(any()));
    });

    test('stop does not turn the next play into a reconnection', () async {
      await engine.setSource(trackUrl, isLive: false);
      clearInteractions(player);

      await engine.stop();
      await engine.play();

      verifyNever(() => player.setUrl(any()));
    });
  });

  test('a new source clears a pending reconnection', () async {
    await engine.setSource(liveUrl, isLive: true);
    await engine.pause();

    await engine.setSource(trackUrl, isLive: false);
    clearInteractions(player);
    await engine.play();

    // setSource already opened the new URL; play must not reopen it.
    verifyNever(() => player.setUrl(any()));
    verify(player.play).called(1);
  });
}
