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

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
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
