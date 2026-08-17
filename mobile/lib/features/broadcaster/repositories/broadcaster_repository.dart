import 'dart:convert';

import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart' show MediaType;

import '../../../core/api/api_client.dart' show ApiException;
import '../../player/models/live_stream.dart';
import '../models/track.dart';

/// Everything the broadcaster screen needs from the API: managing its own
/// streams and its own uploaded tracks.
///
/// Like [StreamRepository] it talks to [http.Client] directly rather than
/// through `ApiClient`: ticket S1 (PR #16) is rewriting that file wholesale,
/// and multipart uploads would need a dedicated method there anyway.
/// [ApiException] is reused so error handling stays uniform app-wide.
class BroadcasterRepository {
  BroadcasterRepository({required this.baseUrl, http.Client? client}) : _client = client ?? http.Client();

  final String baseUrl;
  final http.Client _client;

  /// Creates a stream owned by the authenticated user. It starts offline.
  Future<LiveStream> createStream({
    required String token,
    required String title,
    String description = '',
  }) async {
    final res = await _client.post(
      Uri.parse('$baseUrl/api/v1/streams'),
      headers: _jsonHeaders(token),
      body: jsonEncode({'title': title, 'description': description}),
    );
    return LiveStream.fromJson(_decodeMap(res));
  }

  Future<void> deleteStream({required String token, required String streamId}) async {
    final res = await _client.delete(
      Uri.parse('$baseUrl/api/v1/streams/$streamId'),
      headers: _authHeaders(token),
    );
    _ensureSuccess(res);
  }

  /// Uploads an audio file.
  ///
  /// The part's own Content-Type is set explicitly: the server validates it
  /// against its accepted-formats list before it even looks at the bytes, so
  /// omitting it would get every upload rejected.
  Future<Track> uploadTrack({
    required String token,
    required String title,
    required String artist,
    required PickedAudio audio,
  }) async {
    final request = http.MultipartRequest('POST', Uri.parse('$baseUrl/api/v1/tracks'))
      ..headers.addAll(_authHeaders(token))
      ..fields['title'] = title
      ..fields['artist'] = artist
      ..files.add(http.MultipartFile.fromBytes(
        'file',
        audio.bytes,
        filename: audio.filename,
        contentType: _parseMediaType(audio.contentType),
      ));

    final streamed = await _client.send(request);
    final res = await http.Response.fromStream(streamed);
    return Track.fromJson(_decodeMap(res));
  }

  /// The authenticated broadcaster's own uploads.
  Future<List<Track>> fetchMyTracks(String token) async {
    final res = await _client.get(
      Uri.parse('$baseUrl/api/v1/tracks/mine'),
      headers: _authHeaders(token),
    );
    final decoded = _decodeList(res);
    return decoded.map((e) => Track.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<void> deleteTrack({required String token, required String trackId}) async {
    final res = await _client.delete(
      Uri.parse('$baseUrl/api/v1/tracks/$trackId'),
      headers: _authHeaders(token),
    );
    _ensureSuccess(res);
  }

  /// Absolute URL of a track's audio, for the player or the broadcast source.
  String trackAudioUrl(Track track) => '$baseUrl${track.audioUrl}';

  /// The endpoint a broadcast is published to.
  String publishUrl(String streamId) => '$baseUrl/api/v1/streams/$streamId/publish';

  Map<String, String> _authHeaders(String token) => {'Authorization': 'Bearer $token'};

  Map<String, String> _jsonHeaders(String token) => {
        'Content-Type': 'application/json',
        ..._authHeaders(token),
      };

  Map<String, dynamic> _decodeMap(http.Response res) {
    _ensureSuccess(res);
    if (res.body.isEmpty) return <String, dynamic>{};
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  List<dynamic> _decodeList(http.Response res) {
    _ensureSuccess(res);
    if (res.body.isEmpty) return const [];
    return jsonDecode(res.body) as List<dynamic>;
  }

  void _ensureSuccess(http.Response res) {
    if (res.statusCode >= 200 && res.statusCode < 300) return;
    throw ApiException(res.statusCode, _errorMessage(res.body));
  }

  String _errorMessage(String body) {
    if (body.isEmpty) return 'unknown error';
    try {
      final decoded = jsonDecode(body);
      if (decoded is Map<String, dynamic>) {
        return decoded['error'] as String? ?? 'unknown error';
      }
    } on FormatException {
      // Not JSON (a proxy error page, for instance) — fall through.
    }
    return 'unknown error';
  }
}

/// Splits "audio/mpeg" into the type/subtype pair a multipart part needs.
/// Returns null for a malformed value, which lets `http` fall back to its
/// own default rather than crashing the upload.
MediaType? _parseMediaType(String contentType) {
  final parts = contentType.split('/');
  if (parts.length != 2 || parts[0].isEmpty || parts[1].isEmpty) return null;
  return MediaType(parts[0], parts[1]);
}
