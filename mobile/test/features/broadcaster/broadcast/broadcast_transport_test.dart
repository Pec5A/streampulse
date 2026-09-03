import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:streampulse/features/broadcaster/broadcast/audio_file_picker.dart';
import 'package:streampulse/features/broadcaster/broadcast/broadcast_transport.dart';

/// Captures what a StreamedRequest actually sent, which MockClient cannot do
/// (it materialises the body before handing it over).
class _CapturingClient extends http.BaseClient {
  final chunks = <List<int>>[];
  final completer = Completer<void>();
  http.BaseRequest? request;

  /// How many connections were opened. A track change must not add one.
  int sendCount = 0;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    sendCount++;
    this.request = request;
    await request.finalize().forEach(chunks.add);
    if (!completer.isCompleted) completer.complete();
    return http.StreamedResponse(
      Stream.value(utf8.encode('{"stream_id":"s1","bytes_published":0}')),
      200,
    );
  }

  List<int> get body => chunks.expand((c) => c).toList();
}

void main() {
  group('pacedSource', () {
    test('emits every byte, in chunks of the requested size', () async {
      final source = Stream<List<int>>.fromIterable([
        List<int>.generate(10, (i) => i),
        List<int>.generate(10, (i) => 10 + i),
      ]);

      final emitted = await pacedSource(
        source,
        chunkSize: 4,
        delay: (_) async {}, // no real waiting in tests
      ).toList();

      expect(emitted.expand((c) => c).toList(), List<int>.generate(20, (i) => i));
      // 5 chunks of 4 bytes, nothing left over.
      expect(emitted.map((c) => c.length).toList(), [4, 4, 4, 4, 4]);
    });

    test('flushes a partial trailing chunk', () async {
      final source = Stream<List<int>>.value(List<int>.generate(10, (i) => i));

      final emitted = await pacedSource(source, chunkSize: 4, delay: (_) async {}).toList();

      expect(emitted.map((c) => c.length).toList(), [4, 4, 2]);
      expect(emitted.expand((c) => c).toList(), List<int>.generate(10, (i) => i));
    });

    test('waits between chunks at the requested rate', () async {
      // Without pacing, a four-minute track would be pushed down the socket
      // in seconds and the "live" broadcast would be over before anyone
      // tuned in.
      final waits = <Duration>[];
      final source = Stream<List<int>>.value(List<int>.filled(8, 0));

      await pacedSource(
        source,
        chunkSize: 4,
        bytesPerSecond: 8,
        delay: (d) async => waits.add(d),
      ).toList();

      expect(waits, hasLength(2));
      // 4 bytes at 8 B/s = 500 ms per chunk.
      expect(waits.first, const Duration(milliseconds: 500));
    });

    test('handles an empty source', () async {
      final emitted = await pacedSource(
        const Stream<List<int>>.empty(),
        delay: (_) async {},
      ).toList();
      expect(emitted, isEmpty);
    });
  });

  group('mpegBytesPerSecond', () {
    test('reads a constant 128 kbps stream', () {
      expect(mpegBytesPerSecond(_mp3(bitrateIndex: 9, frames: 3)), 16000);
    });

    test('reads a constant 192 kbps stream', () {
      // The rate that exposed the bug: paced at the old fixed 16 KiB/s, a
      // file like this reaches the listener at two thirds of playback speed.
      expect(mpegBytesPerSecond(_mp3(bitrateIndex: 11, frames: 3)), 24000);
    });

    test('prefers the Xing average over the frame header for a VBR file', () {
      // The first frame says 128 kbps; the Xing block says the file really
      // averages 245 kbps. Pacing on the header would starve the listener
      // for the whole broadcast.
      final bytes = _mp3(bitrateIndex: 9, frames: 3, xingFrames: 1000, xingBytes: 800000);

      // 1000 frames x 1152 samples / 44100 Hz = 26.12 s for 800000 bytes.
      expect(mpegBytesPerSecond(bytes), 30625);
    });

    test('finds the audio behind an ID3v2 tag', () {
      // Cover art routinely pushes the first frame past 100 KiB. A scan that
      // gave up early would fall back on every tagged file — which is most
      // of them.
      final bytes = [..._id3(60000), ..._mp3(bitrateIndex: 11, frames: 3)];
      expect(mpegBytesPerSecond(bytes), 24000);
    });

    test('returns null for bytes that are not MPEG audio', () {
      expect(mpegBytesPerSecond(List<int>.filled(4096, 0x41)), isNull);
    });

    test('returns null when the stream is shorter than one frame', () {
      // A header alone is not proof: the caller must keep feeding rather than
      // pace the whole broadcast on four bytes that merely look right.
      final truncated = _mp3(bitrateIndex: 9, frames: 1).sublist(0, 100);
      expect(mpegBytesPerSecond(truncated), isNull);
    });

    test('ignores a sync pattern that no real frame follows', () {
      // 0xFF 0xFB occurs by chance inside cover art. Accepting the first
      // match would give a confident wrong answer.
      final decoy = <int>[0xFF, 0xFB, 0x90, 0x00, ...List<int>.filled(200, 0x00)];
      expect(mpegBytesPerSecond(decoy), isNull);
    });
  });

  group('pacedSource rate detection', () {
    test('paces a 192 kbps file at its own rate, not the fallback', () async {
      final waits = <Duration>[];

      await pacedSource(
        Stream<List<int>>.value(_mp3(bitrateIndex: 11, frames: 40)),
        chunkSize: 4096,
        delay: (d) async => waits.add(d),
      ).toList();

      // 24000 B/s plus the 8% headroom: 4096 bytes take 158 ms, against the
      // 250 ms the old fixed 16 KiB/s imposed.
      expect(waits, isNotEmpty);
      expect(waits.first, const Duration(microseconds: 158025));
    });

    test('falls back to 16 KiB/s when the bytes name no rate', () async {
      final waits = <Duration>[];

      await pacedSource(
        Stream<List<int>>.value(List<int>.filled(9000, 0x41)),
        chunkSize: 4096,
        delay: (d) async => waits.add(d),
      ).toList();

      // 16 KiB/s is 250 ms per 4096 bytes; the buffering margin applies to the
      // fallback too, since it is just as much a rate we chose.
      expect(fallbackBytesPerSecond, 16 * 1024);
      expect(waits.first, const Duration(microseconds: 231481));
    });

    test('stops running ahead once the buffer is banked', () async {
      // The margin must not accumulate for the length of the broadcast: the
      // connection now stays open across track changes, so an unbounded lead
      // would leave listeners minutes behind the broadcaster.
      final waits = <Duration>[];

      await pacedSource(
        Stream<List<int>>.value(_mp3(bitrateIndex: 9, frames: 7000)),
        chunkSize: 4096,
        delay: (d) async => waits.add(d),
      ).toList();

      // 16000 B/s: 256 ms of audio per chunk, sent in 237 ms while building.
      expect(waits.first, const Duration(microseconds: 237037));
      expect(waits.last, const Duration(microseconds: 256000));

      final banked = waits
          .map((w) => 256000 - w.inMicroseconds)
          .fold<int>(0, (a, b) => a + b);
      expect(banked / 1000000, closeTo(maxLeadSeconds, 0.05));
    });

    test('an explicit rate wins over what the bytes say', () async {
      final waits = <Duration>[];

      await pacedSource(
        Stream<List<int>>.value(_mp3(bitrateIndex: 11, frames: 40)),
        chunkSize: 4096,
        bytesPerSecond: 8192,
        delay: (d) async => waits.add(d),
      ).toList();

      expect(waits.first, const Duration(milliseconds: 500));
    });

    test('still emits every byte once the rate is detected', () async {
      final source = _mp3(bitrateIndex: 9, frames: 10);

      final emitted = await pacedSource(
        Stream<List<int>>.value(source),
        chunkSize: 512,
        delay: (_) async {},
      ).toList();

      expect(emitted.expand((c) => c).toList(), source);
    });
  });

  group('HttpBroadcastTransport', () {
    test('streams the source to the publish endpoint with the bearer token', () async {
      final client = _CapturingClient();
      final transport = HttpBroadcastTransport(client: client);

      await transport.start(
        publishUrl: 'http://api.test/api/v1/streams/s1/publish',
        token: 'jwt-token',
        source: Stream.fromIterable([
          [1, 2, 3],
          [4, 5, 6],
        ]),
      );
      await client.completer.future;

      expect(client.request!.method, 'POST');
      expect(client.request!.url.path, '/api/v1/streams/s1/publish');
      expect(client.request!.headers['Authorization'], 'Bearer jwt-token');
      expect(client.body, [1, 2, 3, 4, 5, 6]);
      // No declared length: the client uses chunked transfer encoding, which
      // is what the publish handler reads.
      expect(client.request!.contentLength, isNull);

      await transport.stop();
    });

    test('reports whether a broadcast is running', () async {
      final client = _CapturingClient();
      final transport = HttpBroadcastTransport(client: client);
      expect(transport.isBroadcasting, isFalse);

      final controller = StreamController<List<int>>();
      await transport.start(
        publishUrl: 'http://api.test/p',
        token: 't',
        source: controller.stream,
      );
      expect(transport.isBroadcasting, isTrue);

      await transport.stop();
      expect(transport.isBroadcasting, isFalse);
      await controller.close();
    });

    test('refuses to start a second broadcast on top of a running one', () async {
      final client = _CapturingClient();
      final transport = HttpBroadcastTransport(client: client);
      final controller = StreamController<List<int>>();

      await transport.start(publishUrl: 'http://api.test/p', token: 't', source: controller.stream);

      await expectLater(
        transport.start(publishUrl: 'http://api.test/p', token: 't', source: const Stream.empty()),
        throwsA(isA<StateError>()),
      );

      await transport.stop();
      await controller.close();
    });

    test('switching source reuses the open connection', () async {
      // Closing the publish request flips the stream offline and ends every
      // listener's response: changing record would disconnect the audience.
      final client = _CapturingClient();
      final transport = HttpBroadcastTransport(client: client);
      final first = StreamController<List<int>>();
      final second = StreamController<List<int>>();

      await transport.start(
        publishUrl: 'http://api.test/api/v1/streams/s1/publish',
        token: 'jwt-token',
        source: first.stream,
      );
      first.add([1, 2, 3]);
      await Future<void>.delayed(Duration.zero);

      await transport.switchSource(second.stream);
      // The abandoned source must go silent rather than interleave with the
      // new one.
      first.add([9, 9]);
      second.add([4, 5, 6]);
      await Future<void>.delayed(Duration.zero);
      await second.close();
      await client.completer.future;

      expect(client.sendCount, 1);
      expect(client.body, [1, 2, 3, 4, 5, 6]);
      await first.close();
    });

    test('switching does not wait on a stalled source', () async {
      // Cancelling a paced source parked on its own input never completes
      // until that input moves. Waiting on it would freeze a track change for
      // as long as the download was stalled.
      final stalled = StreamController<List<int>>();
      final second = StreamController<List<int>>();
      final client = _CapturingClient();
      final transport = HttpBroadcastTransport(client: client);

      await transport.start(
        publishUrl: 'http://api.test/api/v1/streams/s1/publish',
        token: 'jwt-token',
        source: pacedSource(stalled.stream, delay: (_) async {}),
      );
      stalled.add(List<int>.filled(8192, 1));
      await Future<void>.delayed(Duration.zero);

      final outcome = await Future.any([
        transport.switchSource(second.stream).then((_) => 'switched'),
        Future<String>.delayed(const Duration(seconds: 2), () => 'blocked'),
      ]);

      expect(outcome, 'switched');
      await second.close();
      await client.completer.future;
      await stalled.close();
    });

    test('a retired source can no longer reach the connection', () async {
      // Its cancellation is not waited on, so it may still be delivering when
      // the next source starts; those bytes must not interleave in the audio.
      final first = StreamController<List<int>>();
      final second = StreamController<List<int>>();
      final client = _CapturingClient();
      final transport = HttpBroadcastTransport(client: client);

      await transport.start(
        publishUrl: 'http://api.test/api/v1/streams/s1/publish',
        token: 'jwt-token',
        source: first.stream,
      );
      first.add([1, 2, 3]);
      await Future<void>.delayed(Duration.zero);

      await transport.switchSource(second.stream);
      first.add([9, 9, 9]);
      second.add([4, 5, 6]);
      await Future<void>.delayed(Duration.zero);
      await second.close();
      await client.completer.future;

      expect(client.body, [1, 2, 3, 4, 5, 6]);
      await first.close();
    });

    test('switching with nothing on air is refused', () async {
      final transport = HttpBroadcastTransport(client: _CapturingClient());
      expect(
        () => transport.switchSource(const Stream<List<int>>.empty()),
        throwsStateError,
      );
    });

    test('stop on an idle transport is a no-op', () async {
      final transport = HttpBroadcastTransport(client: _CapturingClient());
      await transport.stop(); // must not throw
      expect(transport.isBroadcasting, isFalse);
    });
  });

  group('FilePickerAudioPicker.contentTypeFor', () {
    test('maps every extension the picker offers', () {
      // A mismatch with the backend's accepted list means a guaranteed 415,
      // so the two are pinned together here.
      expect(FilePickerAudioPicker.contentTypeFor('a.mp3'), 'audio/mpeg');
      expect(FilePickerAudioPicker.contentTypeFor('a.aac'), 'audio/aac');
      expect(FilePickerAudioPicker.contentTypeFor('a.m4a'), 'audio/mp4');
      expect(FilePickerAudioPicker.contentTypeFor('a.ogg'), 'audio/ogg');
      expect(FilePickerAudioPicker.contentTypeFor('a.opus'), 'audio/opus');
      expect(FilePickerAudioPicker.contentTypeFor('a.flac'), 'audio/flac');
      expect(FilePickerAudioPicker.contentTypeFor('a.wav'), 'audio/wav');
    });

    test('is case-insensitive', () {
      expect(FilePickerAudioPicker.contentTypeFor('SONG.MP3'), 'audio/mpeg');
    });

    test('falls back to octet-stream, which the server rejects clearly', () {
      expect(FilePickerAudioPicker.contentTypeFor('a.txt'), 'application/octet-stream');
      expect(FilePickerAudioPicker.contentTypeFor('noextension'), 'application/octet-stream');
      expect(FilePickerAudioPicker.contentTypeFor('trailing.'), 'application/octet-stream');
    });
  });
}

/// Builds MPEG-1 Layer III frames at 44.1 kHz, stereo.
///
/// [bitrateIndex] is the index in the layer's bitrate table: 9 is 128 kbps,
/// 11 is 192 kbps. When [xingFrames] is given, a Xing block is written into
/// the first frame so a variable-bitrate file can be simulated.
List<int> _mp3({
  required int bitrateIndex,
  required int frames,
  int? xingFrames,
  int? xingBytes,
}) {
  const bitrates = [0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320];
  final length = 144 * bitrates[bitrateIndex] * 1000 ~/ 44100;

  final out = <int>[];
  for (var f = 0; f < frames; f++) {
    final frame = <int>[
      0xFF, // sync
      0xFB, // sync + MPEG1 + Layer III + no CRC
      (bitrateIndex << 4), // bitrate index, 44.1 kHz, no padding
      0x00, // stereo
    ];
    if (f == 0 && xingFrames != null && xingBytes != null) {
      frame.addAll(List<int>.filled(32, 0)); // side info, MPEG1 stereo
      frame.addAll('Xing'.codeUnits);
      frame.addAll(_be32(0x3)); // flags: frame count + byte count present
      frame.addAll(_be32(xingFrames));
      frame.addAll(_be32(xingBytes));
    }
    frame.addAll(List<int>.filled(length - frame.length, 0));
    out.addAll(frame);
  }
  return out;
}

/// An ID3v2 tag of [payload] bytes, filled with a byte that is not a sync.
List<int> _id3(int payload) => [
      0x49, 0x44, 0x33, // "ID3"
      0x04, 0x00, // version
      0x00, // no footer
      // Syncsafe size: seven bits per byte.
      (payload >> 21) & 0x7f, (payload >> 14) & 0x7f, (payload >> 7) & 0x7f, payload & 0x7f,
      ...List<int>.filled(payload, 0x5A),
    ];

List<int> _be32(int v) => [(v >> 24) & 0xff, (v >> 16) & 0xff, (v >> 8) & 0xff, v & 0xff];
