import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../audio/audio_engine.dart';
import '../audio/just_audio_engine.dart';
import '../audio/stream_audio_handler.dart';
import '../bloc/player_bloc.dart';
import '../models/live_stream.dart';
import '../repositories/stream_repository.dart';

/// Full playback controls for one broadcast.
class PlayerScreen extends StatelessWidget {
  const PlayerScreen({
    super.key,
    required this.stream,
    required this.repository,
    this.engine,
  });

  final LiveStream stream;
  final StreamRepository repository;

  /// The shared background-capable engine. When null — tests, or a preview
  /// with no audio_service running — a plain local engine is built instead.
  final AudioEngine? engine;

  @override
  Widget build(BuildContext context) {
    return BlocProvider(
      create: (_) {
        final audio = engine ?? JustAudioEngine();
        if (audio is StreamAudioHandler) {
          // Name the broadcast on the lock screen and in the notification.
          audio.describe(stream);
        }
        return PlayerBloc(engine: audio)
          ..add(PlayerStreamSelected(stream: stream, url: repository.listenUrl(stream.id)));
      },
      child: _PlayerView(stream: stream),
    );
  }
}

class _PlayerView extends StatelessWidget {
  const _PlayerView({required this.stream});

  final LiveStream stream;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(stream.title)),
      body: BlocBuilder<PlayerBloc, PlayerStateData>(
        builder: (context, state) {
          final bloc = context.read<PlayerBloc>();

          return Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(Icons.podcasts, size: 96, color: Theme.of(context).colorScheme.primary),
                const SizedBox(height: 16),
                Text(stream.title, style: Theme.of(context).textTheme.headlineSmall, textAlign: TextAlign.center),
                if (stream.broadcasterUsername.isNotEmpty)
                  Text(stream.broadcasterUsername, style: Theme.of(context).textTheme.bodyMedium),
                const SizedBox(height: 24),
                _StatusLine(state: state),
                const SizedBox(height: 16),
                _SeekBar(state: state, onSeek: (p) => bloc.add(PlayerSeekRequested(p))),
                const SizedBox(height: 16),
                _TransportControls(state: state),
                const SizedBox(height: 24),
                _VolumeSlider(state: state, onChanged: (v) => bloc.add(PlayerVolumeChanged(v))),
                if (state.errorMessage != null) ...[
                  const SizedBox(height: 16),
                  Text(
                    state.errorMessage!,
                    style: TextStyle(color: Theme.of(context).colorScheme.error),
                    textAlign: TextAlign.center,
                  ),
                ],
              ],
            ),
          );
        },
      ),
    );
  }
}

class _StatusLine extends StatelessWidget {
  const _StatusLine({required this.state});

  final PlayerStateData state;

  @override
  Widget build(BuildContext context) {
    final label = switch (true) {
      _ when state.interrupted => 'Interrompu (appel en cours)',
      _ when state.isLoading => 'Connexion au direct…',
      _ when state.isPlaying => 'En cours de lecture',
      _ when state.status == PlaybackStatus.paused => 'En pause',
      _ when state.status == PlaybackStatus.completed => 'Diffusion terminée',
      _ => 'Prêt',
    };

    return Semantics(
      liveRegion: true,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          if (state.isLoading)
            const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2)),
          if (state.isLoading) const SizedBox(width: 8),
          Text(label),
        ],
      ),
    );
  }
}

class _SeekBar extends StatelessWidget {
  const _SeekBar({required this.state, required this.onSeek});

  final PlayerStateData state;
  final ValueChanged<Duration> onSeek;

  @override
  Widget build(BuildContext context) {
    // A live broadcast has no beginning to scrub back to, so the bar is
    // disabled rather than hidden — the control stays where the user expects
    // it and explains itself.
    if (!state.canSeek) {
      return Semantics(
        label: 'Lecture en direct, navigation impossible',
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.sensors, size: 16),
            const SizedBox(width: 6),
            Text('Direct · ${_format(state.position)}'),
          ],
        ),
      );
    }

    final total = state.duration!;
    final value = state.position.inMilliseconds.clamp(0, total.inMilliseconds).toDouble();

    return Column(
      children: [
        Slider(
          value: value,
          max: total.inMilliseconds.toDouble(),
          label: _format(state.position),
          onChanged: (v) => onSeek(Duration(milliseconds: v.round())),
        ),
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [Text(_format(state.position)), Text(_format(total))],
        ),
      ],
    );
  }

  static String _format(Duration d) {
    final minutes = d.inMinutes.remainder(60).toString().padLeft(2, '0');
    final seconds = d.inSeconds.remainder(60).toString().padLeft(2, '0');
    return d.inHours > 0 ? '${d.inHours}:$minutes:$seconds' : '$minutes:$seconds';
  }
}

class _TransportControls extends StatelessWidget {
  const _TransportControls({required this.state});

  final PlayerStateData state;

  @override
  Widget build(BuildContext context) {
    final bloc = context.read<PlayerBloc>();

    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        IconButton(
          iconSize: 40,
          icon: const Icon(Icons.stop),
          tooltip: 'Arrêter',
          onPressed: state.hasStream ? () => bloc.add(const PlayerStopRequested()) : null,
        ),
        const SizedBox(width: 16),
        IconButton.filled(
          iconSize: 56,
          icon: Icon(state.isPlaying ? Icons.pause : Icons.play_arrow),
          tooltip: state.isPlaying ? 'Mettre en pause' : 'Lire',
          onPressed: state.hasStream ? () => bloc.add(const PlayerPlayPauseToggled()) : null,
        ),
      ],
    );
  }
}

class _VolumeSlider extends StatelessWidget {
  const _VolumeSlider({required this.state, required this.onChanged});

  final PlayerStateData state;
  final ValueChanged<double> onChanged;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Icon(state.volume == 0 ? Icons.volume_off : Icons.volume_up, semanticLabel: 'Volume'),
        Expanded(
          child: Slider(
            value: state.volume,
            label: '${(state.volume * 100).round()} %',
            onChanged: onChanged,
          ),
        ),
      ],
    );
  }
}
