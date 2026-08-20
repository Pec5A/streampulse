import 'package:equatable/equatable.dart';

/// A single entry in a playlist's queue (see backend TrackResponse).
class TrackModel extends Equatable {
  const TrackModel({
    required this.id,
    required this.title,
    required this.artist,
    required this.durationSeconds,
    required this.sourceUrl,
    required this.position,
  });

  factory TrackModel.fromJson(Map<String, dynamic> json) => TrackModel(
        id: json['id'] as String,
        title: json['title'] as String,
        artist: (json['artist'] as String?) ?? '',
        durationSeconds: (json['duration_seconds'] as int?) ?? 0,
        sourceUrl: (json['source_url'] as String?) ?? '',
        position: (json['position'] as int?) ?? 0,
      );

  final String id;
  final String title;
  final String artist;
  final int durationSeconds;
  final String sourceUrl;
  final int position;

  @override
  List<Object?> get props => [id, title, artist, durationSeconds, sourceUrl, position];
}

/// A user-owned playlist. [tracks] is populated on detail reads and empty in
/// list views (see backend PlaylistResponse vs PlaylistWithTracksResponse).
class PlaylistModel extends Equatable {
  const PlaylistModel({
    required this.id,
    required this.ownerId,
    required this.name,
    required this.description,
    required this.isPublic,
    this.tracks = const [],
  });

  factory PlaylistModel.fromJson(Map<String, dynamic> json) => PlaylistModel(
        id: json['id'] as String,
        ownerId: (json['owner_id'] as String?) ?? '',
        name: json['name'] as String,
        description: (json['description'] as String?) ?? '',
        isPublic: (json['is_public'] as bool?) ?? false,
        tracks: ((json['tracks'] as List<dynamic>?) ?? const [])
            .map((e) => TrackModel.fromJson(e as Map<String, dynamic>))
            .toList(),
      );

  final String id;
  final String ownerId;
  final String name;
  final String description;
  final bool isPublic;
  final List<TrackModel> tracks;

  PlaylistModel copyWith({List<TrackModel>? tracks}) => PlaylistModel(
        id: id,
        ownerId: ownerId,
        name: name,
        description: description,
        isPublic: isPublic,
        tracks: tracks ?? this.tracks,
      );

  @override
  List<Object?> get props => [id, ownerId, name, description, isPublic, tracks];
}
