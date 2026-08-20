import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../bloc/playlist_detail_bloc.dart';
import '../models/playlist_model.dart';

/// Shows one playlist's queue with drag-and-drop reordering, plus add/remove.
/// The enclosing [PlaylistDetailBloc] is provided (and seeded) by the caller.
class PlaylistDetailScreen extends StatelessWidget {
  const PlaylistDetailScreen({super.key, required this.playlistName});
  final String playlistName;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(playlistName)),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => _showAddTrackDialog(context),
        icon: const Icon(Icons.add),
        label: const Text('Ajouter un titre'),
      ),
      body: BlocConsumer<PlaylistDetailBloc, PlaylistDetailState>(
        listenWhen: (previous, current) => current is PlaylistDetailFailure,
        listener: (context, state) {
          if (state is PlaylistDetailFailure) {
            ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(state.message)));
          }
        },
        builder: (context, state) => switch (state) {
          PlaylistDetailLoading() => const Center(child: CircularProgressIndicator()),
          PlaylistDetailFailure(:final message) => _ErrorRetry(
              message: message,
              onRetry: () => context.read<PlaylistDetailBloc>().add(const PlaylistDetailRequested()),
            ),
          PlaylistDetailLoaded(:final playlist) => _TrackQueue(playlist: playlist),
        },
      ),
    );
  }

  Future<void> _showAddTrackDialog(BuildContext context) async {
    final titleController = TextEditingController();
    final artistController = TextEditingController();
    final bloc = context.read<PlaylistDetailBloc>();
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Ajouter un titre'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: titleController,
              autofocus: true,
              decoration: const InputDecoration(labelText: 'Titre'),
            ),
            TextField(
              controller: artistController,
              decoration: const InputDecoration(labelText: 'Artiste (optionnel)'),
            ),
          ],
        ),
        actions: [
          TextButton(onPressed: () => Navigator.of(dialogContext).pop(false), child: const Text('Annuler')),
          FilledButton(onPressed: () => Navigator.of(dialogContext).pop(true), child: const Text('Ajouter')),
        ],
      ),
    );
    final title = titleController.text.trim();
    final artist = artistController.text.trim();
    titleController.dispose();
    artistController.dispose();
    if (confirmed == true && title.isNotEmpty) {
      bloc.add(TrackAdded(title: title, artist: artist));
    }
  }
}

class _TrackQueue extends StatelessWidget {
  const _TrackQueue({required this.playlist});
  final PlaylistModel playlist;

  @override
  Widget build(BuildContext context) {
    final tracks = playlist.tracks;
    if (tracks.isEmpty) {
      return const Center(child: Text("La file d'attente est vide"));
    }
    return ReorderableListView.builder(
      itemCount: tracks.length,
      // onReorderItem already adjusts newIndex for the item removed at oldIndex,
      // so it maps directly to the bloc's remove-then-insert reorder.
      onReorderItem: (oldIndex, newIndex) {
        context.read<PlaylistDetailBloc>().add(TracksReordered(oldIndex: oldIndex, newIndex: newIndex));
      },
      itemBuilder: (context, i) {
        final t = tracks[i];
        return Semantics(
          key: ValueKey(t.id),
          label: 'Titre ${i + 1} sur ${tracks.length} : ${t.title}',
          child: ListTile(
            leading: ReorderableDragStartListener(
              index: i,
              child: const Icon(Icons.drag_handle),
            ),
            title: Text(t.title),
            subtitle: t.artist.isEmpty ? null : Text(t.artist),
            trailing: IconButton(
              icon: const Icon(Icons.remove_circle_outline),
              tooltip: 'Retirer de la playlist',
              onPressed: () => context.read<PlaylistDetailBloc>().add(TrackRemoved(t.id)),
            ),
          ),
        );
      },
    );
  }
}

class _ErrorRetry extends StatelessWidget {
  const _ErrorRetry({required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;
  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(message),
          const SizedBox(height: 8),
          OutlinedButton(onPressed: onRetry, child: const Text('Réessayer')),
        ],
      ),
    );
  }
}
