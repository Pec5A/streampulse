import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../audio/audio_engine.dart';
import '../bloc/streams_bloc.dart';
import '../models/live_stream.dart';
import '../repositories/stream_repository.dart';
import 'player_screen.dart';

/// Lists what is on air and opens the player for the tapped broadcast.
class LiveStreamsScreen extends StatelessWidget {
  const LiveStreamsScreen({super.key, required this.repository, this.engine});

  final StreamRepository repository;

  /// The app-wide background-capable engine, passed down to the player.
  /// Null in tests, where the player builds its own.
  final AudioEngine? engine;

  @override
  Widget build(BuildContext context) {
    return BlocProvider(
      create: (_) => StreamsBloc(repository: repository)..add(const StreamsRequested()),
      child: _LiveStreamsView(repository: repository, engine: engine),
    );
  }
}

class _LiveStreamsView extends StatelessWidget {
  const _LiveStreamsView({required this.repository, this.engine});

  final StreamRepository repository;
  final AudioEngine? engine;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('En direct'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            tooltip: 'Rafraîchir',
            // Silent: the list is already on screen, so it is updated in
            // place rather than replaced by a spinner.
            onPressed: () => context.read<StreamsBloc>().add(const StreamsRefreshRequested()),
          ),
        ],
      ),
      body: BlocBuilder<StreamsBloc, StreamsState>(
        builder: (context, state) {
          return switch (state) {
            StreamsInitial() || StreamsLoading() => const Center(child: CircularProgressIndicator()),
            StreamsFailure(:final message) => _ErrorView(
                message: message,
                onRetry: () => context.read<StreamsBloc>().add(const StreamsRequested()),
              ),
            StreamsLoaded(:final streams) when streams.isEmpty => const _EmptyView(),
            StreamsLoaded(:final streams) => RefreshIndicator(
                onRefresh: () async => context.read<StreamsBloc>().add(const StreamsRefreshRequested()),
                child: ListView.separated(
                  itemCount: streams.length,
                  separatorBuilder: (_, _) => const Divider(height: 1),
                  itemBuilder: (context, i) => _StreamTile(
                    stream: streams[i],
                    onTap: () => _openPlayer(context, streams[i]),
                  ),
                ),
              ),
          };
        },
      ),
    );
  }

  void _openPlayer(BuildContext context, LiveStream stream) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => PlayerScreen(stream: stream, repository: repository, engine: engine),
      ),
    );
  }
}

class _StreamTile extends StatelessWidget {
  const _StreamTile({required this.stream, required this.onTap});

  final LiveStream stream;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final listeners = '${stream.listenerCount} '
        '${stream.listenerCount == 1 ? "auditeur" : "auditeurs"}';

    return ListTile(
      onTap: onTap,
      leading: Icon(
        stream.isLive ? Icons.podcasts : Icons.radio,
        color: stream.isLive ? Theme.of(context).colorScheme.error : null,
        // The colour alone must not carry the "live" meaning — the label
        // below and this semantic description say it too.
        semanticLabel: stream.isLive ? 'En direct' : 'Hors ligne',
      ),
      title: Text(stream.title),
      subtitle: Text(
        stream.broadcasterUsername.isEmpty
            ? listeners
            : '${stream.broadcasterUsername} · $listeners',
      ),
      trailing: stream.isLive
          ? const Chip(label: Text('LIVE'), visualDensity: VisualDensity.compact)
          : null,
    );
  }
}

class _EmptyView extends StatelessWidget {
  const _EmptyView();

  @override
  Widget build(BuildContext context) {
    return ListView(
      children: const [
        SizedBox(height: 120),
        Icon(Icons.radio, size: 64),
        SizedBox(height: 16),
        Center(child: Text('Personne ne diffuse pour le moment.')),
      ],
    );
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.message, required this.onRetry});

  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.cloud_off, size: 48),
            const SizedBox(height: 12),
            const Text('Impossible de charger les diffusions.'),
            const SizedBox(height: 4),
            Text(message, textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodySmall),
            const SizedBox(height: 16),
            FilledButton(onPressed: onRetry, child: const Text('Réessayer')),
          ],
        ),
      ),
    );
  }
}
