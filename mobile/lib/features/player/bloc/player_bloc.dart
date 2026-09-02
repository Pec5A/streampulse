import 'dart:async';

import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../audio/audio_engine.dart';
import '../models/live_stream.dart';

part 'player_event.dart';
part 'player_state.dart';

/// Drives playback of a live broadcast.
///
/// The bloc owns the *decisions* (what a pause means, when to resume after a
/// phone call, whether seeking is allowed); [AudioEngine] owns the mechanics.
/// That split is what makes the interruption logic — the part that is
/// genuinely awkward to reproduce on a real device — unit testable.
class PlayerBloc extends Bloc<PlayerEvent, PlayerStateData> {
  PlayerBloc({required AudioEngine engine})
      : _engine = engine,
        super(const PlayerStateData()) {
    on<PlayerStreamSelected>(_onStreamSelected);
    on<PlayerPlayRequested>(_onPlay);
    on<PlayerPauseRequested>(_onPause);
    on<PlayerPlayPauseToggled>(_onToggle);
    on<PlayerStopRequested>(_onStop);
    on<PlayerVolumeChanged>(_onVolume);
    on<PlayerSeekRequested>(_onSeek);
    on<PlayerEngineStatusChanged>(_onEngineStatus);
    on<PlayerPositionChanged>(_onPosition);
    on<PlayerDurationChanged>(_onDuration);
    on<PlayerInterrupted>(_onInterrupted);

    _statusSub = _engine.statusStream.listen((s) => add(PlayerEngineStatusChanged(s)));
    _positionSub = _engine.positionStream.listen((p) => add(PlayerPositionChanged(p)));
    _durationSub = _engine.durationStream.listen((d) => add(PlayerDurationChanged(d)));
    _interruptionSub = _engine.interruptionStream.listen((e) => add(PlayerInterrupted(e)));
  }

  final AudioEngine _engine;

  late final StreamSubscription<PlaybackStatus> _statusSub;
  late final StreamSubscription<Duration> _positionSub;
  late final StreamSubscription<Duration?> _durationSub;
  late final StreamSubscription<InterruptionEvent> _interruptionSub;

  Future<void> _onStreamSelected(PlayerStreamSelected event, Emitter<PlayerStateData> emit) async {
    emit(state.copyWith(
      stream: event.stream,
      status: PlaybackStatus.loading,
      position: Duration.zero,
      clearDuration: true,
      clearError: true,
      interrupted: false,
    ));
    try {
      await _engine.setSource(event.url, isLive: event.stream.isLive);
      await _engine.play();
    } on Object catch (e) {
      emit(state.copyWith(status: PlaybackStatus.idle, errorMessage: _describe(e)));
    }
  }

  Future<void> _onPlay(PlayerPlayRequested event, Emitter<PlayerStateData> emit) async {
    if (!state.hasStream) return;
    try {
      await _engine.play();
      emit(state.copyWith(clearError: true, interrupted: false));
    } on Object catch (e) {
      emit(state.copyWith(errorMessage: _describe(e)));
    }
  }

  Future<void> _onPause(PlayerPauseRequested event, Emitter<PlayerStateData> emit) async {
    if (!state.hasStream) return;
    try {
      await _engine.pause();
    } on Object catch (e) {
      emit(state.copyWith(errorMessage: _describe(e)));
    }
  }

  Future<void> _onToggle(PlayerPlayPauseToggled event, Emitter<PlayerStateData> emit) async {
    if (!state.hasStream) return;
    if (state.isPlaying) {
      add(const PlayerPauseRequested());
    } else {
      add(const PlayerPlayRequested());
    }
  }

  Future<void> _onStop(PlayerStopRequested event, Emitter<PlayerStateData> emit) async {
    await _engine.stop();
    emit(const PlayerStateData());
  }

  Future<void> _onVolume(PlayerVolumeChanged event, Emitter<PlayerStateData> emit) async {
    final volume = event.volume.clamp(0.0, 1.0);
    await _engine.setVolume(volume);
    emit(state.copyWith(volume: volume));
  }

  Future<void> _onSeek(PlayerSeekRequested event, Emitter<PlayerStateData> emit) async {
    // Refusing rather than silently ignoring: seeking a live stream is a UI
    // bug, and the state carries canSeek so the widget can disable the bar.
    if (!state.canSeek) return;

    final clamped = event.position < Duration.zero
        ? Duration.zero
        : (event.position > state.duration! ? state.duration! : event.position);
    await _engine.seek(clamped);
    emit(state.copyWith(position: clamped));
  }

  void _onEngineStatus(PlayerEngineStatusChanged event, Emitter<PlayerStateData> emit) {
    emit(state.copyWith(status: event.status));
  }

  void _onPosition(PlayerPositionChanged event, Emitter<PlayerStateData> emit) {
    emit(state.copyWith(position: event.position));
  }

  void _onDuration(PlayerDurationChanged event, Emitter<PlayerStateData> emit) {
    emit(event.duration == null
        ? state.copyWith(clearDuration: true)
        : state.copyWith(duration: event.duration));
  }

  Future<void> _onInterrupted(PlayerInterrupted event, Emitter<PlayerStateData> emit) async {
    switch (event.event.kind) {
      case InterruptionKind.begin:
        // Pause through the engine so the OS sees us yield audio focus.
        await _engine.pause();
        emit(state.copyWith(interrupted: true));
      case InterruptionKind.end:
        emit(state.copyWith(interrupted: false));
        if (event.event.shouldResume && state.hasStream) {
          await _engine.play();
        }
    }
  }

  String _describe(Object error) => error.toString();

  @override
  Future<void> close() async {
    await _statusSub.cancel();
    await _positionSub.cancel();
    await _durationSub.cancel();
    await _interruptionSub.cancel();
    await _engine.dispose();
    return super.close();
  }
}
