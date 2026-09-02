import 'package:flutter_test/flutter_test.dart';
import 'package:streampulse/features/player/models/live_stream.dart';

void main() {
  group('LiveStream.fromJson', () {
    test('reads the full API payload', () {
      final stream = LiveStream.fromJson({
        'id': 's1',
        'title': 'Jazz de nuit',
        'description': 'session live',
        'broadcaster_id': 'u1',
        'broadcaster_username': 'kaysz',
        'status': 'live',
        'listener_count': 12,
      });

      expect(stream.id, 's1');
      expect(stream.title, 'Jazz de nuit');
      expect(stream.description, 'session live');
      expect(stream.broadcasterId, 'u1');
      expect(stream.broadcasterUsername, 'kaysz');
      expect(stream.listenerCount, 12);
      expect(stream.isLive, isTrue);
    });

    test('falls back sensibly on the optional fields', () {
      // broadcaster_username is omitted by the API when the join finds
      // nothing (a deleted account), so it must not be required.
      final stream = LiveStream.fromJson({'id': 's1'});

      expect(stream.title, '');
      expect(stream.broadcasterUsername, '');
      expect(stream.status, 'offline');
      expect(stream.listenerCount, 0);
      expect(stream.isLive, isFalse);
    });

    test('accepts a listener count decoded as a double', () {
      final stream = LiveStream.fromJson({'id': 's1', 'listener_count': 3.0});
      expect(stream.listenerCount, 3);
    });

    test('is offline for any status other than live', () {
      expect(LiveStream.fromJson({'id': 's1', 'status': 'offline'}).isLive, isFalse);
      expect(LiveStream.fromJson({'id': 's1', 'status': 'live'}).isLive, isTrue);
    });
  });

  test('equality is value based', () {
    final a = LiveStream.fromJson({'id': 's1', 'title': 'x'});
    final b = LiveStream.fromJson({'id': 's1', 'title': 'x'});
    final c = LiveStream.fromJson({'id': 's1', 'title': 'y'});

    expect(a, equals(b));
    expect(a, isNot(equals(c)));
  });
}
