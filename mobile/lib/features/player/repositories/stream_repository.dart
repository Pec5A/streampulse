import 'dart:convert';

import 'package:http/http.dart' as http;

import '../../../core/api/api_client.dart' show ApiException;
import '../models/live_stream.dart';

/// Reads the stream catalogue and builds the URL the audio engine plays.
///
/// It talks to [http.Client] directly rather than going through [ApiClient]:
/// ApiClient currently only exposes `post`, and ticket S1 (PR #16) is
/// rewriting that whole file to add `get`/`getList`. Adding my own `getList`
/// on top would guarantee a conflict on a shared file for no benefit. Once
/// #16 lands, `_get` here collapses into a call to `ApiClient.getList`.
/// [ApiException] is reused as-is so error handling stays uniform app-wide.
class StreamRepository {
  StreamRepository({required this.baseUrl, http.Client? client}) : _client = client ?? http.Client();

  final String baseUrl;
  final http.Client _client;

  /// Streams currently being broadcast.
  Future<List<LiveStream>> fetchLiveStreams() async {
    final decoded = await _getList('/api/v1/streams/live');
    return decoded.map((e) => LiveStream.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// Every stream, live or not, for the browse screen.
  Future<List<LiveStream>> fetchAllStreams() async {
    final decoded = await _getList('/api/v1/streams');
    return decoded.map((e) => LiveStream.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// The endless chunked-audio endpoint handed straight to the audio engine.
  ///
  /// No token: listening is public, so the player never has to attach a
  /// credential to a long-lived audio connection.
  String listenUrl(String streamId) => '$baseUrl/api/v1/streams/$streamId/listen';

  Future<List<dynamic>> _getList(String path) async {
    final res = await _client.get(Uri.parse('$baseUrl$path'));
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw ApiException(res.statusCode, _errorMessage(res.body));
    }
    if (res.body.isEmpty) return const [];
    return jsonDecode(res.body) as List<dynamic>;
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
