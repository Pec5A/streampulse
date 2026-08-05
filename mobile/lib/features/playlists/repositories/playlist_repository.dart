import '../../../core/api/api_client.dart';
import '../../../core/storage/secure_storage.dart';
import '../models/playlist_model.dart';

/// Talks to the playlists API, attaching the stored JWT to every request.
class PlaylistRepository {
  PlaylistRepository({required ApiClient apiClient, required SecureStorage storage})
      : _api = apiClient,
        _storage = storage;

  final ApiClient _api;
  final SecureStorage _storage;

  static const _base = '/api/v1/playlists';

  Future<List<PlaylistModel>> list() async {
    final json = await _api.getList(_base, token: await _storage.readToken());
    return json.map((e) => PlaylistModel.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<PlaylistModel> get(String id) async {
    final json = await _api.get('$_base/$id', token: await _storage.readToken());
    return PlaylistModel.fromJson(json);
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
