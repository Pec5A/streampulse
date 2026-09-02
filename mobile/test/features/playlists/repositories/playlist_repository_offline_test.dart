import 'dart:io';

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

PlaylistRepository repoWith(http.Client client, {PlaylistCache cache = const PlaylistCache()}) {
  final storage = MockSecureStorage();
  when(() => storage.readToken()).thenAnswer((_) async => null);
  return PlaylistRepository(
    apiClient: ApiClient(baseUrl: 'http://x', client: client),
    storage: storage,
    cache: cache,
  );
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  test('list() serves the cached snapshot on a ClientException (offline)', () async {
    const cache = PlaylistCache();
    const cached = PlaylistModel(id: 'p1', ownerId: 'u', name: 'Cached Mix', description: '', isPublic: false);
    await cache.savePlaylists(const [cached]);

    final offline = MockClient((_) async => throw http.ClientException('offline'));
    expect(await repoWith(offline, cache: cache).list(), const [cached]);
  });

  test('list() serves the cached snapshot on a SocketException (offline)', () async {
    const cache = PlaylistCache();
    const cached = PlaylistModel(id: 'p1', ownerId: 'u', name: 'Cached Mix', description: '', isPublic: false);
    await cache.savePlaylists(const [cached]);

    final offline = MockClient((_) async => throw const SocketException('no route to host'));
    expect(await repoWith(offline, cache: cache).list(), const [cached]);
  });

  test('list() caches the response so a later offline read returns it', () async {
    final online = MockClient(
      (_) async => http.Response('[{"id":"p2","owner_id":"u","name":"Live","description":"","is_public":false}]', 200),
    );
    const cache = PlaylistCache();
    final result = await repoWith(online, cache: cache).list();
    expect(result.single.name, 'Live');
    expect((await cache.readPlaylists()).single.id, 'p2');
  });

  test('list() does not mask a 500 with a JSON body', () async {
    final failing = MockClient((_) async => http.Response('{"error":"boom"}', 500));
    expect(() => repoWith(failing).list(), throwsA(isA<ApiException>()));
  });

  test('list() does not mask a 500 with a non-JSON body (the review bug)', () async {
    // Prime the cache to prove it is NOT served when the server errors.
    await const PlaylistCache().savePlaylists(
      const [PlaylistModel(id: 'stale', ownerId: 'u', name: 'stale', description: '', isPublic: false)],
    );
    // A reverse-proxy HTML error page — not valid JSON. Previously this raised a
    // FormatException that the generic catch swallowed, silently serving cache.
    final proxyError = MockClient((_) async => http.Response('<html>502 Bad Gateway</html>', 500));
    await expectLater(repoWith(proxyError).list(), throwsA(isA<ApiException>()));
  });
}
