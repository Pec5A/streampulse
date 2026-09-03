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

  /// Swaps the bytes being broadcast without closing the connection.
  ///
  /// Changing record must not take the station off the air: closing the publish
  /// request flips the stream offline and ends every listener's response, so a
  /// track change done by stop-then-start disconnects the audience. Feeding a
  /// new source into the same open request changes only what they hear.
  Future<void> switchSource(Stream<List<int>> source);

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
    _pump(request, source);
  }

  @override
  Future<void> switchSource(Stream<List<int>> source) async {
    final request = _request;
    if (request == null) {
      throw StateError('no broadcast to switch');
    }
    // Cancelling rather than letting the old source finish: its onDone would
    // close the sink, which is exactly the disconnection this avoids.
    await _subscription?.cancel();
    _pump(request, source);
  }

  /// Feeds [source] into the open request until it ends or is replaced.
  void _pump(http.StreamedRequest request, Stream<List<int>> source) {
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

/// The rate used until the stream names its own, and when it never does.
/// 16 KiB/s is a 128 kbps stream — the most common encoding, and so the
/// least wrong guess available when the bytes cannot be read.
const int fallbackBytesPerSecond = 16 * 1024;

/// How far to look for the first frame header before giving up. An ID3v2 tag
/// carrying cover art routinely runs past 100 KiB and the audio starts only
/// after it, so a small window would fall back on every tagged file. Bounded
/// all the same: a source that is not audio must not grow this forever.
const int _sniffLimit = 512 * 1024;

/// How far ahead of real time the audio is sent.
///
/// Feeding a listener at exactly playback speed leaves their buffer no way to
/// recover: it never grows, so the first network hiccup becomes a stall that
/// lasts the rest of the broadcast. A little margin lets the buffer refill
/// after a hiccup instead of draining for good. Small on purpose — the more
/// margin, the further ahead of the music the broadcast finishes.
const double pacingHeadroom = 1.08;

/// Emits [source] at the rate the audio is meant to be played, in [chunkSize]
/// pieces.
///
/// Without this, "broadcasting a track" would push the whole file down the
/// socket as fast as the network allows: a four-minute song would be over in
/// two seconds, and listeners joining a moment later would find nothing left.
/// Pacing makes the broadcast behave like a radio station.
///
/// The rate is read from the audio itself. A fixed rate under-feeds anything
/// encoded above it — a 194 kbps file paced at 128 kbps reaches the listener
/// at 67% of playback speed, so the buffer drains continuously: the player
/// stutters, and the broadcast outlasts the music by half its length.
///
/// Pass [bytesPerSecond] to override the detection; it is what tests use, and
/// it wins when the caller knows something the bytes do not say.
Stream<List<int>> pacedSource(
  Stream<List<int>> source, {
  int? bytesPerSecond,
  int chunkSize = 4 * 1024,
  Future<void> Function(Duration) delay = _wait,
}) async* {
  var rate = bytesPerSecond ?? fallbackBytesPerSecond;
  var sniffing = bytesPerSecond == null;
  final head = <int>[];
  final buffer = <int>[];

  await for (final bytes in source) {
    buffer.addAll(bytes);

    if (sniffing) {
      head.addAll(bytes);
      final detected = mpegBytesPerSecond(head);
      if (detected != null) {
        rate = (detected * pacingHeadroom).round();
        sniffing = false;
      } else if (head.length >= _sniffLimit) {
        // Not audio we can read. The fallback stands rather than stalling a
        // broadcast over a header we will never find.
        sniffing = false;
      }
      if (!sniffing) head.clear();
    }

    while (buffer.length >= chunkSize) {
      yield buffer.sublist(0, chunkSize);
      buffer.removeRange(0, chunkSize);
      // Recomputed per chunk: detection can land after the first bytes are
      // already on their way.
      await delay(Duration(microseconds: (chunkSize * 1000000 / rate).round()));
    }
  }
  if (buffer.isNotEmpty) {
    yield List<int>.from(buffer);
  }
}

/// Reads the playback rate of MPEG audio out of [head], in bytes per second,
/// or null when those bytes carry no header it can trust.
///
/// Skips an ID3v2 tag if one opens the stream, then looks for a frame header.
/// A constant-bitrate file is described by that header alone; a variable one
/// carries a Xing/Info block inside the first frame holding the average the
/// header cannot express, and that average is what pacing needs.
///
/// Returns null rather than a guess when the header is absent, unreadable, or
/// not yet fully arrived — the caller keeps feeding until it can decide.
int? mpegBytesPerSecond(List<int> head) {
  for (var i = _id3Length(head); i + 4 <= head.length; i++) {
    final frame = _parseFrame(head, i);
    if (frame == null) continue;

    // Four plausible bytes are not a frame. Believe the header only once the
    // frame it describes is fully present and the next one begins exactly
    // where this one says it ends — sync bytes occur by chance in cover art.
    final next = i + frame.frameLength;
    if (next + 4 > head.length || _parseFrame(head, next) == null) continue;

    return _xingBytesPerSecond(head, i, frame) ?? frame.bitrateBps ~/ 8;
  }
  return null;
}

/// Total size of the ID3v2 tag opening [b], or 0 when there is none.
int _id3Length(List<int> b) {
  if (b.length < 10 || b[0] != 0x49 || b[1] != 0x44 || b[2] != 0x33) return 0;
  // Syncsafe integer: seven bits per byte, top bit always clear, precisely so
  // a tag's length can never contain a false frame sync.
  final size = ((b[6] & 0x7f) << 21) | ((b[7] & 0x7f) << 14) | ((b[8] & 0x7f) << 7) | (b[9] & 0x7f);
  final footer = (b[5] & 0x10) != 0 ? 10 : 0;
  return 10 + size + footer;
}

/// One MPEG audio frame header, decoded.
class _Frame {
  const _Frame({
    required this.bitrateBps,
    required this.sampleRate,
    required this.frameLength,
    required this.samplesPerFrame,
    required this.layer,
    required this.mpeg1,
    required this.mono,
  });

  final int bitrateBps;
  final int sampleRate;
  final int frameLength;
  final int samplesPerFrame;
  final int layer;
  final bool mpeg1;
  final bool mono;
}

const List<List<int>> _mpeg1Bitrates = [
  [0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0], // I
  [0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0], // II
  [0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0], // III
];

const List<List<int>> _mpeg2Bitrates = [
  [0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0], // I
  [0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0], // II
  [0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0], // III
];

/// Keyed by the header's version bits: 3 = MPEG1, 2 = MPEG2, 0 = MPEG2.5.
const Map<int, List<int>> _sampleRates = {
  3: [44100, 48000, 32000],
  2: [22050, 24000, 16000],
  0: [11025, 12000, 8000],
};

/// Decodes the four header bytes at [i], or null if they are not a valid one.
_Frame? _parseFrame(List<int> b, int i) {
  if (i + 4 > b.length) return null;
  if (b[i] != 0xFF || (b[i + 1] & 0xE0) != 0xE0) return null;

  final versionBits = (b[i + 1] >> 3) & 0x3;
  final layerBits = (b[i + 1] >> 1) & 0x3;
  if (versionBits == 1 || layerBits == 0) return null; // reserved values

  final bitrateIndex = (b[i + 2] >> 4) & 0xF;
  final sampleIndex = (b[i + 2] >> 2) & 0x3;
  // Index 0 is "free format" — the rate is not in the stream at all — and 15
  // is invalid. Neither can be paced, so neither is accepted.
  if (bitrateIndex == 0 || bitrateIndex == 15 || sampleIndex == 3) return null;

  final mpeg1 = versionBits == 3;
  final layer = 4 - layerBits;
  final kbps = (mpeg1 ? _mpeg1Bitrates : _mpeg2Bitrates)[layer - 1][bitrateIndex];
  final sampleRate = _sampleRates[versionBits]![sampleIndex];
  if (kbps == 0) return null;

  final bps = kbps * 1000;
  final padding = (b[i + 2] >> 1) & 0x1;
  final samplesPerFrame = layer == 1
      ? 384
      : layer == 2
          ? 1152
          : (mpeg1 ? 1152 : 576);
  // Layer I counts its length in four-byte slots; the others in single bytes.
  final frameLength = layer == 1
      ? ((12 * bps ~/ sampleRate) + padding) * 4
      : (samplesPerFrame ~/ 8 * bps) ~/ sampleRate + padding;
  if (frameLength <= 4) return null;

  return _Frame(
    bitrateBps: bps,
    sampleRate: sampleRate,
    frameLength: frameLength,
    samplesPerFrame: samplesPerFrame,
    layer: layer,
    mpeg1: mpeg1,
    mono: ((b[i + 3] >> 6) & 0x3) == 3,
  );
}

/// Average rate declared by a Xing/Info block inside the frame at [start].
///
/// This is what makes a variable-bitrate file pace correctly: its first frame
/// header names one frame's rate, often the lowest in the file, which would
/// under-feed the listener for the whole broadcast.
int? _xingBytesPerSecond(List<int> b, int start, _Frame frame) {
  if (frame.layer != 3) return null;
  // The block sits after the side information, whose size depends on version
  // and channel count.
  final sideInfo = frame.mpeg1 ? (frame.mono ? 17 : 32) : (frame.mono ? 9 : 17);
  final tag = start + 4 + sideInfo;
  if (tag + 16 > b.length) return null;

  final name = String.fromCharCodes(b, tag, tag + 4);
  if (name != 'Xing' && name != 'Info') return null;

  // Both counts are needed, and requiring them keeps the field offsets fixed:
  // the byte count only sits at +12 when the frame count precedes it. Without
  // both, the frame header remains the better answer.
  final flags = _uint32(b, tag + 4);
  if (flags & 0x1 == 0 || flags & 0x2 == 0) return null;

  final frames = _uint32(b, tag + 8);
  final bytes = _uint32(b, tag + 12);
  if (frames <= 0 || bytes <= 0) return null;

  final seconds = frames * frame.samplesPerFrame / frame.sampleRate;
  return seconds <= 0 ? null : (bytes / seconds).round();
}

int _uint32(List<int> b, int i) => (b[i] << 24) | (b[i + 1] << 16) | (b[i + 2] << 8) | b[i + 3];

Future<void> _wait(Duration d) => Future<void>.delayed(d);
