import 'package:equatable/equatable.dart';

/// An uploaded audio file, as the API describes it.
///
/// `storage_key` is intentionally absent from the API payload, so there is
/// nothing to model here: the client reaches audio through [audioUrl] only.
class Track extends Equatable {
  const Track({
    required this.id,
    required this.title,
    required this.artist,
    required this.contentType,
    required this.sizeBytes,
    required this.uploaderId,
    required this.uploaderUsername,
    required this.audioUrl,
  });

  factory Track.fromJson(Map<String, dynamic> json) {
    return Track(
      id: json['id'] as String,
      title: json['title'] as String? ?? '',
      artist: json['artist'] as String? ?? '',
      contentType: json['content_type'] as String? ?? '',
      sizeBytes: (json['size_bytes'] as num?)?.toInt() ?? 0,
      uploaderId: json['uploader_id'] as String? ?? '',
      uploaderUsername: json['uploader_username'] as String? ?? '',
      audioUrl: json['audio_url'] as String? ?? '',
    );
  }

  final String id;
  final String title;
  final String artist;
  final String contentType;
  final int sizeBytes;
  final String uploaderId;
  final String uploaderUsername;

  /// Path relative to the API host, e.g. `/api/v1/tracks/<id>/audio`.
  final String audioUrl;

  /// Human-readable size for the broadcaster's track list.
  String get displaySize {
    if (sizeBytes >= 1 << 20) return '${(sizeBytes / (1 << 20)).toStringAsFixed(1)} Mo';
    if (sizeBytes >= 1 << 10) return '${(sizeBytes / (1 << 10)).toStringAsFixed(0)} Ko';
    return '$sizeBytes o';
  }

  @override
  List<Object?> get props => [id, title, artist, contentType, sizeBytes, uploaderId, uploaderUsername, audioUrl];
}

/// An audio file chosen on the device, ready to upload.
///
/// Bytes rather than a path: it keeps the upload path identical on mobile,
/// desktop and web, where there is no filesystem path to speak of.
class PickedAudio extends Equatable {
  const PickedAudio({required this.filename, required this.bytes, required this.contentType});

  final String filename;
  final List<int> bytes;
  final String contentType;

  @override
  List<Object?> get props => [filename, bytes.length, contentType];
}
