import 'package:file_picker/file_picker.dart';

import '../models/track.dart';

/// Chooses an audio file on the device.
///
/// A port, like every other platform dependency in this slice: `file_picker`
/// goes through platform channels, so a bloc depending on it directly could
/// only be tested on a device. Behind this interface, "the user cancelled"
/// and "the user picked an unsupported file" are two lines in a unit test.
abstract class AudioFilePicker {
  /// Returns the chosen file, or null if the user cancelled.
  Future<PickedAudio?> pick();
}

/// The real picker, restricted to the extensions the API accepts.
class FilePickerAudioPicker implements AudioFilePicker {
  const FilePickerAudioPicker();

  /// Kept in step with `acceptedAudioTypes` on the backend — offering an
  /// extension here that the server rejects is a guaranteed failed upload.
  static const contentTypesByExtension = {
    'mp3': 'audio/mpeg',
    'aac': 'audio/aac',
    'm4a': 'audio/mp4',
    'ogg': 'audio/ogg',
    'opus': 'audio/opus',
    'flac': 'audio/flac',
    'wav': 'audio/wav',
  };

  @override
  Future<PickedAudio?> pick() async {
    final file = await FilePicker.pickFile(
      type: FileType.custom,
      allowedExtensions: contentTypesByExtension.keys.toList(),
    );
    if (file == null) return null;

    // Bytes rather than a path: identical behaviour on mobile, desktop and
    // web, where there is no filesystem path to open.
    final bytes = await file.readAsBytes();

    return PickedAudio(
      filename: file.name,
      bytes: bytes,
      contentType: contentTypeFor(file.name),
    );
  }

  /// Maps a filename to the Content-Type the upload part declares.
  ///
  /// Falls back to `application/octet-stream`, which the server rejects with
  /// a clear 415 — better than guessing `audio/mpeg` and having it stored
  /// under the wrong type.
  static String contentTypeFor(String filename) {
    final dot = filename.lastIndexOf('.');
    if (dot < 0 || dot == filename.length - 1) return 'application/octet-stream';
    final extension = filename.substring(dot + 1).toLowerCase();
    return contentTypesByExtension[extension] ?? 'application/octet-stream';
  }
}
