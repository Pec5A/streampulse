import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:streampulse/features/playlists/cache/playlist_cache.dart';
import 'package:streampulse/features/playlists/models/playlist_model.dart';

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  const playlist = PlaylistModel(
    id: 'p1',
    ownerId: 'u',
    name: 'Mix',
    description: 'chill',
    isPublic: true,
    tracks: [TrackModel(id: 't1', title: 'A', artist: 'x', durationSeconds: 12, sourceUrl: 's', position: 0)],
  );

  test('round-trips the playlists list', () async {
    const cache = PlaylistCache();
    await cache.savePlaylists(const [playlist]);
    expect(await cache.readPlaylists(), const [playlist]);
  });

  test('returns empty when nothing is cached', () async {
    expect(await const PlaylistCache().readPlaylists(), isEmpty);
  });

  test('round-trips a playlist detail and misses cleanly', () async {
    const cache = PlaylistCache();
    await cache.savePlaylist(playlist);
    expect(await cache.readPlaylist('p1'), playlist);
    expect(await cache.readPlaylist('missing'), isNull);
  });
}
