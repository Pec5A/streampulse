import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../bloc/playlist_detail_bloc.dart';
import '../bloc/playlists_bloc.dart';
import '../models/playlist_model.dart';
import '../repositories/playlist_repository.dart';
import 'playlist_detail_screen.dart';

/// Lists the current user's playlists and lets them create a new one. The
/// enclosing [PlaylistsBloc] is provided (and seeded) by the caller.
class PlaylistsScreen extends StatelessWidget {
  const PlaylistsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Mes playlists')),
      floatingActionButton: FloatingActionButton(
        onPressed: () => _showCreateDialog(context),
        tooltip: 'Créer une playlist',
        child: const Icon(Icons.add),
      ),
      body: BlocBuilder<PlaylistsBloc, PlaylistsState>(
        builder: (context, state) => switch (state) {
          PlaylistsInitial() || PlaylistsLoading() => const Center(child: CircularProgressIndicator()),
          PlaylistsFailure(:final message) => _ErrorRetry(
              message: message,
              onRetry: () => context.read<PlaylistsBloc>().add(const PlaylistsRequested()),
            ),
          PlaylistsLoaded(:final playlists) =>
            playlists.isEmpty ? const _EmptyPlaylists() : _PlaylistList(playlists: playlists),
        },
      ),
    );
  }

  Future<void> _showCreateDialog(BuildContext context) async {
    final controller = TextEditingController();
    final bloc = context.read<PlaylistsBloc>();
    final name = await showDialog<String>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Nouvelle playlist'),
        content: TextField(
          controller: controller,
          autofocus: true,
          decoration: const InputDecoration(labelText: 'Nom'),
          onSubmitted: (value) => Navigator.of(dialogContext).pop(value),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.of(dialogContext).pop(), child: const Text('Annuler')),
          FilledButton(
            onPressed: () => Navigator.of(dialogContext).pop(controller.text),
            child: const Text('Créer'),
          ),
        ],
      ),
    );
    controller.dispose();
    if (name != null && name.trim().isNotEmpty) {
      bloc.add(PlaylistCreated(name: name.trim()));
    }
  }
}

class _PlaylistList extends StatelessWidget {
  const _PlaylistList({required this.playlists});
  final List<PlaylistModel> playlists;

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: () async => context.read<PlaylistsBloc>().add(const PlaylistsRequested()),
      child: ListView.separated(
        itemCount: playlists.length,
        separatorBuilder: (_, _) => const Divider(height: 1),
        itemBuilder: (context, i) {
          final p = playlists[i];
          return ListTile(
            leading: const Icon(Icons.queue_music),
            title: Text(p.name),
            subtitle: p.description.isEmpty ? null : Text(p.description),
            trailing: Icon(p.isPublic ? Icons.public : Icons.lock_outline, size: 18),
            onTap: () => _openDetail(context, p),
          );
        },
      ),
    );
  }

  void _openDetail(BuildContext context, PlaylistModel p) {
    final repository = context.read<PlaylistRepository>();
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => BlocProvider(
          create: (_) => PlaylistDetailBloc(repository: repository, playlistId: p.id)
            ..add(const PlaylistDetailRequested()),
          child: PlaylistDetailScreen(playlistName: p.name),
        ),
      ),
    );
  }
}

class _EmptyPlaylists extends StatelessWidget {
  const _EmptyPlaylists();
  @override
  Widget build(BuildContext context) {
    return const Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.queue_music, size: 64),
          SizedBox(height: 8),
          Text('Aucune playlist pour le moment'),
        ],
      ),
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
