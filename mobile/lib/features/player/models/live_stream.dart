import 'package:equatable/equatable.dart';

/// A broadcast as the API describes it.
///
/// Field names mirror the JSON contract pinned by the backend test
/// `TestRouter_StreamJSONShape` — renaming one on either side breaks the app,
/// so both sides assert on it.
class LiveStream extends Equatable {
  const LiveStream({
    required this.id,
    required this.title,
    required this.description,
    required this.broadcasterId,
    required this.broadcasterUsername,
    required this.status,
    required this.listenerCount,
  });

  factory LiveStream.fromJson(Map<String, dynamic> json) {
    return LiveStream(
      id: json['id'] as String,
      title: json['title'] as String? ?? '',
      description: json['description'] as String? ?? '',
      broadcasterId: json['broadcaster_id'] as String? ?? '',
      broadcasterUsername: json['broadcaster_username'] as String? ?? '',
      status: json['status'] as String? ?? 'offline',
      // The API omits nothing here, but a listener count arriving as a
      // double from a JSON decoder would crash a hard `as int` cast.
      listenerCount: (json['listener_count'] as num?)?.toInt() ?? 0,
    );
  }

  final String id;
  final String title;
  final String description;
  final String broadcasterId;
  final String broadcasterUsername;
  final String status;
  final int listenerCount;

  bool get isLive => status == 'live';

  @override
  List<Object?> get props => [id, title, description, broadcasterId, broadcasterUsername, status, listenerCount];
}
