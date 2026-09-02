import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../bloc/broadcaster_bloc.dart';
import '../broadcast/audio_file_picker.dart';
import '../broadcast/broadcast_transport.dart';
import '../models/track.dart';
import '../repositories/broadcaster_repository.dart';

/// The broadcaster console: upload tracks, create a stream, go live.
class BroadcasterScreen extends StatelessWidget {
  const BroadcasterScreen({
    super.key,
    required this.repository,
    required this.token,
    this.picker,
    this.transport,
  });

  final BroadcasterRepository repository;
  final String token;

  /// Injectable so tests (and a desktop build) can swap the platform pieces.
  final AudioFilePicker? picker;
  final BroadcastTransport? transport;

  @override
  Widget build(BuildContext context) {
    return BlocProvider(
      create: (_) => BroadcasterBloc(
        repository: repository,
        picker: picker ?? const FilePickerAudioPicker(),
        transport: transport ?? HttpBroadcastTransport(),
        token: token,
      )..add(const BroadcasterTracksRequested()),
      child: const _BroadcasterView(),
    );
  }
}

class _BroadcasterView extends StatelessWidget {
  const _BroadcasterView();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Diffuser')),
      body: BlocConsumer<BroadcasterBloc, BroadcasterState>(
        listenWhen: (a, b) => a.errorMessage != b.errorMessage && b.errorMessage != null,
        listener: (context, state) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(state.errorMessage!)),
          );
        },
        builder: (context, state) {
          if (state.loading && state.tracks.isEmpty) {
            return const Center(child: CircularProgressIndicator());
          }
          return ListView(
            padding: const EdgeInsets.all(16),
            children: [
              _LiveCard(state: state),
              const SizedBox(height: 24),
              _UploadCard(state: state),
              const SizedBox(height: 24),
              Text('Mes pistes', style: Theme.of(context).textTheme.titleMedium),
              const SizedBox(height: 8),
              if (state.tracks.isEmpty)
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 24),
                  child: Center(child: Text('Aucune piste téléversée pour le moment.')),
                )
              else
                ...state.tracks.map((t) => _TrackTile(track: t, state: state)),
            ],
          );
        },
      ),
    );
  }
}

class _LiveCard extends StatefulWidget {
  const _LiveCard({required this.state});
  final BroadcasterState state;

  @override
  State<_LiveCard> createState() => _LiveCardState();
}

class _LiveCardState extends State<_LiveCard> {
  final _title = TextEditingController();

  @override
  void dispose() {
    _title.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.state;
    final bloc = context.read<BroadcasterBloc>();

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  state.isLive ? Icons.sensors : Icons.sensors_off,
                  color: state.isLive ? Theme.of(context).colorScheme.error : null,
                  // The colour must not be the only carrier of "on air".
                  semanticLabel: state.isLive ? 'En direct' : 'Hors ligne',
                ),
                const SizedBox(width: 8),
                Text(
                  state.isLive ? 'EN DIRECT' : 'Hors ligne',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
              ],
            ),
            const SizedBox(height: 12),
            if (!state.hasStream) ...[
              TextField(
                controller: _title,
                decoration: const InputDecoration(
                  labelText: 'Titre du direct',
                  hintText: 'Jazz de nuit',
                ),
              ),
              const SizedBox(height: 12),
              FilledButton.icon(
                icon: const Icon(Icons.add),
                label: const Text('Créer le direct'),
                onPressed: state.loading
                    ? null
                    : () => bloc.add(BroadcasterStreamCreated(title: _title.text)),
              ),
            ] else ...[
              Text(state.stream!.title, style: Theme.of(context).textTheme.bodyLarge),
              if (state.broadcastingTrack != null)
                Padding(
                  padding: const EdgeInsets.only(top: 4),
                  child: Text('À l\'antenne : ${state.broadcastingTrack!.title}'),
                ),
              const SizedBox(height: 12),
              if (state.isLive)
                FilledButton.icon(
                  style: FilledButton.styleFrom(
                    backgroundColor: Theme.of(context).colorScheme.error,
                  ),
                  icon: const Icon(Icons.stop),
                  label: const Text('Arrêter la diffusion'),
                  onPressed: () => bloc.add(const BroadcasterStopRequested()),
                )
              else
                Text(
                  state.tracks.isEmpty
                      ? 'Téléverse une piste pour pouvoir diffuser.'
                      : 'Choisis une piste ci-dessous pour passer à l\'antenne.',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
            ],
          ],
        ),
      ),
    );
  }
}

class _UploadCard extends StatefulWidget {
  const _UploadCard({required this.state});
  final BroadcasterState state;

  @override
  State<_UploadCard> createState() => _UploadCardState();
}

class _UploadCardState extends State<_UploadCard> {
  final _title = TextEditingController();
  final _artist = TextEditingController();

  @override
  void dispose() {
    _title.dispose();
    _artist.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.state;
    final bloc = context.read<BroadcasterBloc>();
    final pending = state.pendingUpload;

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Téléverser une piste', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 12),
            OutlinedButton.icon(
              icon: const Icon(Icons.attach_file),
              label: Text(pending == null ? 'Choisir un fichier audio' : pending.filename),
              onPressed: state.uploading ? null : () => bloc.add(const BroadcasterTrackPicked()),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _title,
              decoration: const InputDecoration(labelText: 'Titre'),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: _artist,
              decoration: const InputDecoration(labelText: 'Artiste (optionnel)'),
            ),
            const SizedBox(height: 12),
            FilledButton.icon(
              icon: state.uploading
                  ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.cloud_upload),
              label: Text(state.uploading ? 'Téléversement…' : 'Téléverser'),
              onPressed: state.uploading || pending == null
                  ? null
                  : () => bloc.add(BroadcasterUploadRequested(
                        title: _title.text,
                        artist: _artist.text,
                      )),
            ),
          ],
        ),
      ),
    );
  }
}

class _TrackTile extends StatelessWidget {
  const _TrackTile({required this.track, required this.state});

  final Track track;
  final BroadcasterState state;

  @override
  Widget build(BuildContext context) {
    final bloc = context.read<BroadcasterBloc>();
    final onAir = state.broadcastingTrack?.id == track.id && state.isLive;

    return ListTile(
      leading: Icon(onAir ? Icons.graphic_eq : Icons.audiotrack),
      title: Text(track.title),
      subtitle: Text(
        [if (track.artist.isNotEmpty) track.artist, track.displaySize].join(' · '),
      ),
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          IconButton(
            icon: const Icon(Icons.podcasts),
            tooltip: 'Diffuser cette piste',
            onPressed: state.canGoLive ? () => bloc.add(BroadcasterGoLiveRequested(track)) : null,
          ),
          IconButton(
            icon: const Icon(Icons.delete_outline),
            tooltip: 'Supprimer',
            onPressed: onAir ? null : () => bloc.add(BroadcasterTrackDeleted(track.id)),
          ),
        ],
      ),
    );
  }
}
