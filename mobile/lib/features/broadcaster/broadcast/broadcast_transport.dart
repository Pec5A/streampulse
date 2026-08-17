import 'dart:async';

import 'package:http/http.dart' as http;

/// Pushes bytes to the API's publish endpoint for as long as a broadcast
/// lasts.
///
/// A port rather than a concrete client, for the same reason as `AudioEngine`
/// on the player side: the bloc's start/stop state machine is then testable
/// without a network, and swapping the HTTP transport for a WebSocket one
/// (the browser path) touches one file.
abstract class BroadcastTransport {
  bool get isBroadcasting;

  /// Opens the publish connection and streams [source] into it until the
  /// source ends or [stop] is called.
  Future<void> start({
    required String publishUrl,
    required String token,
    required Stream<List<int>> source,
  });

  /// Ends the broadcast. The server flips the stream back to offline as soon
  /// as the request body closes.
  Future<void> stop();
}

/// The real transport: one long-lived chunked POST.
class HttpBroadcastTransport implements BroadcastTransport {
  HttpBroadcastTransport({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  StreamSubscription<List<int>>? _subscription;
  http.StreamedRequest? _request;
  Future<http.StreamedResponse>? _response;

  @override
  bool get isBroadcasting => _request != null;

  @override
  Future<void> start({
    required String publishUrl,
    required String token,
    required Stream<List<int>> source,
  }) async {
    if (isBroadcasting) {
      throw StateError('a broadcast is already running');
    }

    // StreamedRequest with no declared content length: the client uses
    // chunked transfer encoding, which is exactly what the publish handler
    // reads. The server answers only once the body closes — that response is
    // the session summary, not an acknowledgement of the connection.
    final request = http.StreamedRequest('POST', Uri.parse(publishUrl))
      ..headers['Authorization'] = 'Bearer $token'
      ..headers['Content-Type'] = 'application/octet-stream';

    _request = request;
    _response = _client.send(request);

    _subscription = source.listen(
      request.sink.add,
      onDone: () => request.sink.close(),
      onError: (Object _) => request.sink.close(),
      cancelOnError: true,
    );
  }

  @override
  Future<void> stop() async {
    final subscription = _subscription;
    final request = _request;
    final response = _response;

    _subscription = null;
    _request = null;
    _response = null;

    if (subscription == null || request == null) return;

    await subscription.cancel();
    // Closing the sink is what tells the server the broadcast is over.
    await request.sink.close();
    // Drain the response so the connection is released rather than leaked.
    try {
      final res = await response;
      await res?.stream.drain<void>();
    } on Object {
      // The broadcast is over either way; a failure here is not actionable.
    }
  }
}

/// Emits [source] at roughly [bytesPerSecond], in [chunkSize] pieces.
///
/// Without this, "broadcasting a track" would push the whole file down the
/// socket as fast as the network allows: a four-minute song would be over in
/// two seconds, and listeners joining a moment later would find nothing left.
/// Pacing makes the broadcast behave like a radio station.
///
/// The rate is a deliberate approximation — real pacing would decode the
/// bitrate from the container. 16 KiB/s matches a 128 kbps stream, which is
/// what the app uploads in practice.
Stream<List<int>> pacedSource(
  Stream<List<int>> source, {
  int bytesPerSecond = 16 * 1024,
  int chunkSize = 4 * 1024,
  Future<void> Function(Duration) delay = _wait,
}) async* {
  final interval = Duration(microseconds: (chunkSize * 1000000 / bytesPerSecond).round());
  final buffer = <int>[];

  await for (final bytes in source) {
    buffer.addAll(bytes);
    while (buffer.length >= chunkSize) {
      yield buffer.sublist(0, chunkSize);
      buffer.removeRange(0, chunkSize);
      await delay(interval);
    }
  }
  if (buffer.isNotEmpty) {
    yield List<int>.from(buffer);
  }
}

Future<void> _wait(Duration d) => Future<void>.delayed(d);
