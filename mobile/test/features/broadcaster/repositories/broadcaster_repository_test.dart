import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/features/broadcaster/models/track.dart';
import 'package:streampulse/features/broadcaster/repositories/broadcaster_repository.dart';

void main() {
  const baseUrl = 'http://api.test';
  const token = 'jwt-token';

  Map<String, dynamic> trackJson({String id = 't1'}) => {
        'id': id,
        'title': 'Nocturne',
        'artist': 'KaysZ',
        'content_type': 'audio/mpeg',
        'size_bytes': 4096,
        'uploader_id': 'u1',
        'uploader_username': 'kaysz',
        'audio_url': '/api/v1/tracks/$id/audio',
      };

  BroadcasterRepository repoWith(MockClient client) =>
      BroadcasterRepository(baseUrl: baseUrl, client: client);

  group('createStream', () {
    test('posts the payload with the bearer token', () async {
      late http.Request captured;
      final repo = repoWith(MockClient((req) async {
        captured = req;
        return http.Response(
          jsonEncode({'id': 's1', 'title': 'Jazz', 'status': 'offline', 'listener_count': 0}),
          201,
        );
      }));

      final stream = await repo.createStream(token: token, title: 'Jazz', description: 'live');

      expect(captured.method, 'POST');
      expect(captured.url.path, '/api/v1/streams');
      expect(captured.headers['Authorization'], 'Bearer $token');
      expect(jsonDecode(captured.body), {'title': 'Jazz', 'description': 'live'});
      expect(stream.id, 's1');
      expect(stream.isLive, isFalse);
    });

    test('throws ApiException on a rejection', () async {
      final repo = repoWith(MockClient((_) async => http.Response(
            jsonEncode({'error': 'invalid stream: title is required'}),
            400,
          )));

      await expectLater(
        repo.createStream(token: token, title: ''),
        throwsA(isA<ApiException>()
            .having((e) => e.statusCode, 'statusCode', 400)
            .having((e) => e.message, 'message', contains('title is required'))),
      );
    });
  });

  group('uploadTrack', () {
    test('sends a multipart body with the audio content type on the part', () async {
      // The server validates the part's own Content-Type before reading a
      // byte, so getting this wrong means every upload is a 415.
      late String body;
      late Map<String, String> headers;
      final repo = repoWith(MockClient((req) async {
        body = req.body;
        headers = req.headers;
        return http.Response(jsonEncode(trackJson()), 201);
      }));

      final track = await repo.uploadTrack(
        token: token,
        title: 'Nocturne',
        artist: 'KaysZ',
        audio: const PickedAudio(
          filename: 'nocturne.mp3',
          bytes: [1, 2, 3, 4],
          contentType: 'audio/mpeg',
        ),
      );

      expect(headers['Authorization'], 'Bearer $token');
      expect(headers['content-type'], startsWith('multipart/form-data'));
      expect(body, contains('name="title"'));
      expect(body, contains('Nocturne'));
      expect(body, contains('name="artist"'));
      expect(body, contains('name="file"'));
      expect(body, contains('filename="nocturne.mp3"'));
      expect(body, contains('content-type: audio/mpeg'));
      expect(track.id, 't1');
      expect(track.sizeBytes, 4096);
    });

    test('surfaces a 415 from the server', () async {
      final repo = repoWith(MockClient((_) async => http.Response(
            jsonEncode({'error': 'unsupported media type: content is text/html, not audio'}),
            415,
          )));

      await expectLater(
        repo.uploadTrack(
          token: token,
          title: 'x',
          artist: '',
          audio: const PickedAudio(filename: 'a.mp3', bytes: [1], contentType: 'audio/mpeg'),
        ),
        throwsA(isA<ApiException>().having((e) => e.statusCode, 'statusCode', 415)),
      );
    });

    test('surfaces a 413 when the file is too large', () async {
      final repo = repoWith(MockClient((_) async => http.Response(
            jsonEncode({'error': 'file exceeds the 50 MiB limit'}),
            413,
          )));

      await expectLater(
        repo.uploadTrack(
          token: token,
          title: 'x',
          artist: '',
          audio: const PickedAudio(filename: 'a.mp3', bytes: [1], contentType: 'audio/mpeg'),
        ),
        throwsA(isA<ApiException>().having((e) => e.statusCode, 'statusCode', 413)),
      );
    });
  });

  group('fetchMyTracks', () {
    test('calls the authenticated endpoint and parses the list', () async {
      late http.Request captured;
      final repo = repoWith(MockClient((req) async {
        captured = req;
        return http.Response(jsonEncode([trackJson(), trackJson(id: 't2')]), 200);
      }));

      final tracks = await repo.fetchMyTracks(token);

      expect(captured.url.path, '/api/v1/tracks/mine');
      expect(captured.headers['Authorization'], 'Bearer $token');
      expect(tracks, hasLength(2));
      expect(tracks.first.title, 'Nocturne');
    });

    test('returns an empty list rather than null', () async {
      final repo = repoWith(MockClient((_) async => http.Response('[]', 200)));
      expect(await repo.fetchMyTracks(token), isEmpty);
    });

    test('throws on 401 instead of showing an empty library', () async {
      final repo = repoWith(MockClient((_) async => http.Response(
            jsonEncode({'error': 'invalid or expired token'}),
            401,
          )));

      await expectLater(
        repo.fetchMyTracks(token),
        throwsA(isA<ApiException>().having((e) => e.statusCode, 'statusCode', 401)),
      );
    });
  });

  group('deleteTrack / deleteStream', () {
    test('delete a track', () async {
      late http.Request captured;
      final repo = repoWith(MockClient((req) async {
        captured = req;
        return http.Response('', 204);
      }));

      await repo.deleteTrack(token: token, trackId: 't1');

      expect(captured.method, 'DELETE');
      expect(captured.url.path, '/api/v1/tracks/t1');
    });

    test('a 403 is surfaced, not swallowed', () async {
      final repo = repoWith(MockClient((_) async => http.Response(
            jsonEncode({'error': 'not your track'}),
            403,
          )));

      await expectLater(
        repo.deleteTrack(token: token, trackId: 't1'),
        throwsA(isA<ApiException>().having((e) => e.statusCode, 'statusCode', 403)),
      );
    });

    test('delete a stream', () async {
      late http.Request captured;
      final repo = repoWith(MockClient((req) async {
        captured = req;
        return http.Response('', 204);
      }));

      await repo.deleteStream(token: token, streamId: 's1');

      expect(captured.method, 'DELETE');
      expect(captured.url.path, '/api/v1/streams/s1');
    });
  });

  group('url helpers', () {
    final repo = BroadcasterRepository(baseUrl: baseUrl);

    test('trackAudioUrl is absolute', () {
      final track = Track.fromJson(trackJson());
      expect(repo.trackAudioUrl(track), 'http://api.test/api/v1/tracks/t1/audio');
    });

    test('publishUrl points at the chunked publish endpoint', () {
      expect(repo.publishUrl('s1'), 'http://api.test/api/v1/streams/s1/publish');
    });
  });

  group('Track', () {
    test('formats a human-readable size', () {
      Track sized(int bytes) => Track.fromJson({...trackJson(), 'size_bytes': bytes});

      expect(sized(512).displaySize, '512 o');
      expect(sized(4096).displaySize, '4 Ko');
      expect(sized(3 * 1024 * 1024).displaySize, '3.0 Mo');
    });

    test('tolerates a payload missing every optional field', () {
      final track = Track.fromJson({'id': 't1'});
      expect(track.title, '');
      expect(track.artist, '');
      expect(track.sizeBytes, 0);
      expect(track.audioUrl, '');
    });
  });
}
