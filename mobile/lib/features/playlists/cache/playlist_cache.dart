import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

import '../models/playlist_model.dart';

/// Local cache of playlists so they stay viewable offline. Stores structured
/// JSON metadata in SharedPreferences (not media files) — small and enough for
/// the list and already-opened playlists.
class PlaylistCache {
  const PlaylistCache();

  static const _listKey = 'cache.playlists';
  static String _detailKey(String id) => 'cache.playlist.$id';

  Future<void> savePlaylists(List<PlaylistModel> playlists) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_listKey, jsonEncode(playlists.map((p) => p.toJson()).toList()));
  }

  Future<List<PlaylistModel>> readPlaylists() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_listKey);
    if (raw == null) return const [];
    return (jsonDecode(raw) as List<dynamic>)
        .map((e) => PlaylistModel.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> savePlaylist(PlaylistModel playlist) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_detailKey(playlist.id), jsonEncode(playlist.toJson()));
  }

  Future<PlaylistModel?> readPlaylist(String id) async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_detailKey(id));
    if (raw == null) return null;
    return PlaylistModel.fromJson(jsonDecode(raw) as Map<String, dynamic>);
  }
}
