import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/features/playlists/bloc/playlist_detail_bloc.dart';
import 'package:streampulse/features/playlists/models/playlist_model.dart';
import 'package:streampulse/features/playlists/repositories/playlist_repository.dart';

class MockPlaylistRepository extends Mock implements PlaylistRepository {}

const _t1 = TrackModel(id: 't1', title: 'A', artist: '', durationSeconds: 0, sourceUrl: '', position: 0);
const _t2 = TrackModel(id: 't2', title: 'B', artist: '', durationSeconds: 0, sourceUrl: '', position: 1);
const _t3 = TrackModel(id: 't3', title: 'C', artist: '', durationSeconds: 0, sourceUrl: '', position: 2);
const _t4 = TrackModel(id: 't4', title: 'D', artist: '', durationSeconds: 0, sourceUrl: '', position: 3);
const _loaded = PlaylistModel(
  id: 'p1',
  ownerId: 'u',
  name: 'Mix',
  description: '',
  isPublic: false,
  tracks: [_t1, _t2, _t3],
);

// What the server returns after reordering to [t2, t3, t1] (positions renumbered).
const _savedAfterReorder = PlaylistModel(
  id: 'p1',
  ownerId: 'u',
  name: 'Mix',
  description: '',
  isPublic: false,
  tracks: [
    TrackModel(id: 't2', title: 'B', artist: '', durationSeconds: 0, sourceUrl: '', position: 0),
    TrackModel(id: 't3', title: 'C', artist: '', durationSeconds: 0, sourceUrl: '', position: 1),
    TrackModel(id: 't1', title: 'A', artist: '', durationSeconds: 0, sourceUrl: '', position: 2),
  ],
);

void main() {
  setUpAll(() => registerFallbackValue(<String>[]));

  late MockPlaylistRepository repo;
  setUp(() => repo = MockPlaylistRepository());

  PlaylistDetailBloc build() => PlaylistDetailBloc(repository: repo, playlistId: 'p1');

  group('PlaylistDetailBloc', () {
    blocTest<PlaylistDetailBloc, PlaylistDetailState>(
      'emits [Loading, Loaded] on PlaylistDetailRequested',
      setUp: () => when(() => repo.get('p1')).thenAnswer((_) async => _loaded),
      build: build,
      act: (bloc) => bloc.add(const PlaylistDetailRequested()),
      expect: () => const [PlaylistDetailLoading(), PlaylistDetailLoaded(_loaded)],
    );

    blocTest<PlaylistDetailBloc, PlaylistDetailState>(
      'emits [Loading, Failure] when the load fails',
      setUp: () => when(() => repo.get('p1')).thenThrow(ApiException(404, 'not found')),
      build: build,
      act: (bloc) => bloc.add(const PlaylistDetailRequested()),
      expect: () => const [PlaylistDetailLoading(), PlaylistDetailFailure('not found')],
    );

    blocTest<PlaylistDetailBloc, PlaylistDetailState>(
      'adds a track then reloads with the new track',
      setUp: () {
        when(() => repo.addTrack('p1', title: 'D', artist: '')).thenAnswer((_) async => _t4);
        when(() => repo.get('p1')).thenAnswer((_) async => _loaded.copyWith(tracks: const [_t1, _t2, _t3, _t4]));
      },
      seed: () => const PlaylistDetailLoaded(_loaded),
      build: build,
      act: (bloc) => bloc.add(const TrackAdded(title: 'D')),
      expect: () => [PlaylistDetailLoaded(_loaded.copyWith(tracks: const [_t1, _t2, _t3, _t4]))],
      verify: (_) {
        verify(() => repo.addTrack('p1', title: 'D', artist: '')).called(1);
        verify(() => repo.get('p1')).called(1);
      },
    );

    blocTest<PlaylistDetailBloc, PlaylistDetailState>(
      'optimistically reorders (0 -> 2) then persists the server order',
      setUp: () => when(() => repo.reorder('p1', any())).thenAnswer((_) async => _savedAfterReorder),
      seed: () => const PlaylistDetailLoaded(_loaded),
      build: build,
      act: (bloc) => bloc.add(const TracksReordered(oldIndex: 0, newIndex: 2)),
      expect: () => [
        PlaylistDetailLoaded(_loaded.copyWith(tracks: const [_t2, _t3, _t1])),
        const PlaylistDetailLoaded(_savedAfterReorder),
      ],
      verify: (_) {
        final captured = verify(() => repo.reorder('p1', captureAny())).captured.single as List<String>;
        expect(captured, ['t2', 't3', 't1']);
      },
    );
  });
}
