part of 'broadcaster_bloc.dart';

/// One flat state: the track list, the current stream and the broadcast flag
/// all coexist, so splitting them into a sealed hierarchy would mean copying
/// the same fields on every transition.
class BroadcasterState extends Equatable {
  const BroadcasterState({
    this.tracks = const [],
    this.stream,
    this.pendingUpload,
    this.broadcastingTrack,
    this.loading = false,
    this.uploading = false,
    this.starting = false,
    this.isLive = false,
    this.errorMessage,
  });

  final List<Track> tracks;

  /// The stream this broadcaster created, or null before they create one.
  final LiveStream? stream;

  /// A file chosen on the device, not yet uploaded.
  final PickedAudio? pendingUpload;

  /// What is currently going out on air.
  final Track? broadcastingTrack;

  final bool loading;
  final bool uploading;
  final bool starting;
  final bool isLive;
  final String? errorMessage;

  bool get hasStream => stream != null;
  /// Whether a track can be put on air. Deliberately true while already
  /// broadcasting: a station changes record without going off the air, and
  /// forcing a stop first is the reason switching track felt impossible.
  bool get canGoLive => hasStream && !starting && tracks.isNotEmpty;

  BroadcasterState copyWith({
    List<Track>? tracks,
    LiveStream? stream,
    PickedAudio? pendingUpload,
    Track? broadcastingTrack,
    bool? loading,
    bool? uploading,
    bool? starting,
    bool? isLive,
    String? errorMessage,
    bool clearError = false,
    bool clearPendingUpload = false,
    bool clearBroadcastingTrack = false,
  }) {
    return BroadcasterState(
      tracks: tracks ?? this.tracks,
      stream: stream ?? this.stream,
      pendingUpload: clearPendingUpload ? null : (pendingUpload ?? this.pendingUpload),
      broadcastingTrack:
          clearBroadcastingTrack ? null : (broadcastingTrack ?? this.broadcastingTrack),
      loading: loading ?? this.loading,
      uploading: uploading ?? this.uploading,
      starting: starting ?? this.starting,
      isLive: isLive ?? this.isLive,
      errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
    );
  }

  @override
  List<Object?> get props => [
        tracks,
        stream?.id,
        pendingUpload,
        broadcastingTrack?.id,
        loading,
        uploading,
        starting,
        isLive,
        errorMessage,
      ];
}
