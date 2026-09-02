import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/features/player/repositories/stream_repository.dart';

void main() {
  const baseUrl = 'http://api.test';

  StreamRepository repoReturning(
    Object? body, {
    int status = 200,
    void Function(http.Request)? onRequest,
  }) {
    final client = MockClient((request) async {
      onRequest?.call(request);
      return http.Response(
        body is String ? body : jsonEncode(body),
        status,
        headers: {'content-type': 'application/json'},
      );
    });
    return StreamRepository(baseUrl: baseUrl, client: client);
  }

  Map<String, dynamic> streamJson({
    String id = 's1',
    String status = 'live',
    int listeners = 5,
  }) =>
      {
        'id': id,
        'title': 'Jazz de nuit',
        'description': 'session live',
        'broadcaster_id': 'u1',
        'broadcaster_username': 'kaysz',
        'status': status,
        'listener_count': listeners,
        'created_at': '2026-08-17T10:00:00Z',
        'updated_at': '2026-08-17T10:00:00Z',
      };

  group('listenUrl', () {
    test('points at the chunked audio endpoint', () {
      final repo = StreamRepository(baseUrl: baseUrl);
      expect(repo.listenUrl('abc'), 'http://api.test/api/v1/streams/abc/listen');
    });

    test('carries no credential — listening is public', () {
      final repo = StreamRepository(baseUrl: baseUrl);
      expect(repo.listenUrl('abc'), isNot(contains('token')));
    });
  });

  group('fetchLiveStreams', () {
    test('calls the live endpoint and parses the payload', () async {
      String? calledPath;
      final repo = repoReturning(
        [streamJson()],
        onRequest: (r) => calledPath = r.url.path,
      );

      final streams = await repo.fetchLiveStreams();

      expect(calledPath, '/api/v1/streams/live');
      expect(streams, hasLength(1));
      expect(streams.first.id, 's1');
      expect(streams.first.title, 'Jazz de nuit');
      expect(streams.first.broadcasterUsername, 'kaysz');
      expect(streams.first.listenerCount, 5);
      expect(streams.first.isLive, isTrue);
    });

    test('returns an empty list when nobody is broadcasting', () async {
      final repo = repoReturning(<dynamic>[]);
      expect(await repo.fetchLiveStreams(), isEmpty);
    });

    test('handles an empty response body', () async {
      final repo = repoReturning('');
      expect(await repo.fetchLiveStreams(), isEmpty);
    });

    test('throws ApiException carrying the API error message', () async {
      final repo = repoReturning({'error': 'stream operation failed'}, status: 500);

      await expectLater(
        repo.fetchLiveStreams(),
        throwsA(isA<ApiException>()
            .having((e) => e.statusCode, 'statusCode', 500)
            .having((e) => e.message, 'message', 'stream operation failed')),
      );
    });

    test('survives a non-JSON error body from a proxy', () async {
      final repo = repoReturning('<html>502 Bad Gateway</html>', status: 502);

      await expectLater(
        repo.fetchLiveStreams(),
        throwsA(isA<ApiException>()
            .having((e) => e.statusCode, 'statusCode', 502)
            .having((e) => e.message, 'message', 'unknown error')),
      );
    });
  });

  group('fetchAllStreams', () {
    test('calls the catalogue endpoint', () async {
      String? calledPath;
      final repo = repoReturning(
        [streamJson(status: 'offline', listeners: 0)],
        onRequest: (r) => calledPath = r.url.path,
      );

      final streams = await repo.fetchAllStreams();

      expect(calledPath, '/api/v1/streams');
      expect(streams.single.isLive, isFalse);
    });
  });
}
