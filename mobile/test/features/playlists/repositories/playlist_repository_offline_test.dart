import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mocktail/mocktail.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/core/storage/secure_storage.dart';
import 'package:streampulse/features/playlists/cache/playlist_cache.dart';
import 'package:streampulse/features/playlists/models/playlist_model.dart';
import 'package:streampulse/features/playlists/repositories/playlist_repository.dart';

class MockSecureStorage extends Mock implements SecureStorage {}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  test('list() serves the cached snapshot when the network is down', () async {
    const cache = PlaylistCache();
    const cached = PlaylistModel(id: 'p1', ownerId: 'u', name: 'Cached Mix', description: '', isPublic: false);
    await cache.savePlaylists(const [cached]);

    final storage = MockSecureStorage();
    when(() => storage.readToken()).thenAnswer((_) async => 'tok');

    final offline = MockClient((_) async => throw http.ClientException('offline'));
    final repo = PlaylistRepository(
      apiClient: ApiClient(baseUrl: 'http://unreachable', client: offline),
      storage: storage,
      cache: cache,
    );

    expect(await repo.list(), const [cached]);
  });

  test('list() caches the response so a later offline read returns it', () async {
    final storage = MockSecureStorage();
    when(() => storage.readToken()).thenAnswer((_) async => null);

    final online = MockClient(
      (_) async => http.Response('[{"id":"p2","owner_id":"u","name":"Live","description":"","is_public":false}]', 200),
    );
    const cache = PlaylistCache();
    final repo = PlaylistRepository(
      apiClient: ApiClient(baseUrl: 'http://ok', client: online),
      storage: storage,
      cache: cache,
    );

    final result = await repo.list();
    expect(result.single.name, 'Live');
    expect((await cache.readPlaylists()).single.id, 'p2');
  });

  test('list() does not mask a real API error with cache', () async {
    final storage = MockSecureStorage();
    when(() => storage.readToken()).thenAnswer((_) async => null);

    final failing = MockClient((_) async => http.Response('{"error":"boom"}', 500));
    final repo = PlaylistRepository(
      apiClient: ApiClient(baseUrl: 'http://ok', client: failing),
      storage: storage,
      cache: const PlaylistCache(),
    );

    expect(() => repo.list(), throwsA(isA<ApiException>()));
  });
}
