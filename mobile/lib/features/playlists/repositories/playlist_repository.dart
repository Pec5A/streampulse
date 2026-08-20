import 'dart:async';
import 'dart:io';

import 'package:http/http.dart' as http;

import '../../../core/api/api_client.dart';
import '../../../core/storage/secure_storage.dart';
import '../cache/playlist_cache.dart';
import '../models/playlist_model.dart';

/// Talks to the playlists API, attaching the stored JWT to every request.
/// Reads are cached locally so playlists stay available offline (ticket S4).
class PlaylistRepository {
  PlaylistRepository({
    required ApiClient apiClient,
    required SecureStorage storage,
    PlaylistCache? cache,
  })  : _api = apiClient,
        _storage = storage,
        _cache = cache ?? const PlaylistCache();

  final ApiClient _api;
  final SecureStorage _storage;
  final PlaylistCache _cache;

  static const _base = '/api/v1/playlists';

  /// Lists the caller's playlists; only on a genuine connectivity failure does
  /// it fall back to the last cached snapshot. A server error (ApiException) or
  /// a malformed response is propagated, never masked by stale cache.
  Future<List<PlaylistModel>> list() async {
    try {
      final json = await _api.getList(_base, token: await _storage.readToken());
      final playlists = json.map((e) => PlaylistModel.fromJson(e as Map<String, dynamic>)).toList();
      await _cache.savePlaylists(playlists);
      return playlists;
    } catch (e) {
      if (_isOffline(e)) return _cache.readPlaylists();
      rethrow;
    }
  }

  /// Fetches a playlist with its tracks; only on a connectivity failure does it
  /// return the cached copy (if present).
  Future<PlaylistModel> get(String id) async {
    try {
      final json = await _api.get('$_base/$id', token: await _storage.readToken());
      final playlist = PlaylistModel.fromJson(json);
      await _cache.savePlaylist(playlist);
      return playlist;
    } catch (e) {
      if (_isOffline(e)) {
        final cached = await _cache.readPlaylist(id);
        if (cached != null) return cached;
      }
      rethrow;
    }
  }

  Future<PlaylistModel> create(String name, {String description = '', bool isPublic = false}) async {
    final json = await _api.post(
      _base,
      {'name': name, 'description': description, 'is_public': isPublic},
      token: await _storage.readToken(),
    );
    return PlaylistModel.fromJson(json);
  }

  Future<void> delete(String id) async {
    await _api.delete('$_base/$id', token: await _storage.readToken());
  }

  Future<TrackModel> addTrack(
    String playlistId, {
    required String title,
    String artist = '',
    int durationSeconds = 0,
    String sourceUrl = '',
  }) async {
    final json = await _api.post(
      '$_base/$playlistId/tracks',
      {
        'title': title,
        'artist': artist,
        'duration_seconds': durationSeconds,
        'source_url': sourceUrl,
      },
      token: await _storage.readToken(),
    );
    return TrackModel.fromJson(json);
  }

  Future<void> removeTrack(String playlistId, String trackId) async {
    await _api.delete('$_base/$playlistId/tracks/$trackId', token: await _storage.readToken());
  }

  Future<PlaylistModel> reorder(String playlistId, List<String> trackIds) async {
    final json = await _api.put(
      '$_base/$playlistId/tracks/order',
      {'track_ids': trackIds},
      token: await _storage.readToken(),
    );
    return PlaylistModel.fromJson(json);
  }
}

/// Whether [e] is a genuine connectivity failure (device offline, unreachable
/// host, or timeout) — the only case where serving stale cache is correct. A
/// server error (ApiException) or a decoding failure (FormatException) must
/// surface instead of being silently swallowed.
bool _isOffline(Object e) => e is SocketException || e is http.ClientException || e is TimeoutException;
