import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/features/playlists/bloc/playlists_bloc.dart';
import 'package:streampulse/features/playlists/models/playlist_model.dart';
import 'package:streampulse/features/playlists/repositories/playlist_repository.dart';

class MockPlaylistRepository extends Mock implements PlaylistRepository {}

void main() {
  late MockPlaylistRepository repo;
  setUp(() => repo = MockPlaylistRepository());

  const p1 = PlaylistModel(id: 'p1', ownerId: 'u', name: 'A', description: '', isPublic: false);
  const p2 = PlaylistModel(id: 'p2', ownerId: 'u', name: 'B', description: '', isPublic: false);

  group('PlaylistsBloc', () {
    blocTest<PlaylistsBloc, PlaylistsState>(
      'emits [Loading, Loaded] on PlaylistsRequested',
      setUp: () => when(() => repo.list()).thenAnswer((_) async => [p1, p2]),
      build: () => PlaylistsBloc(repository: repo),
      act: (bloc) => bloc.add(const PlaylistsRequested()),
      expect: () => const [
        PlaylistsLoading(),
        PlaylistsLoaded([p1, p2]),
      ],
    );

    blocTest<PlaylistsBloc, PlaylistsState>(
      'emits [Loading, Failure] when the repository throws',
      setUp: () => when(() => repo.list()).thenThrow(ApiException(500, 'server down')),
      build: () => PlaylistsBloc(repository: repo),
      act: (bloc) => bloc.add(const PlaylistsRequested()),
      expect: () => const [
        PlaylistsLoading(),
        PlaylistsFailure('server down'),
      ],
    );

    blocTest<PlaylistsBloc, PlaylistsState>(
      'creates a playlist then reloads the list',
      setUp: () {
        when(() => repo.create('New', description: '')).thenAnswer((_) async => p1);
        when(() => repo.list()).thenAnswer((_) async => [p1]);
      },
      build: () => PlaylistsBloc(repository: repo),
      act: (bloc) => bloc.add(const PlaylistCreated(name: 'New')),
      expect: () => const [
        PlaylistsLoaded([p1]),
      ],
      verify: (_) {
        verify(() => repo.create('New', description: '')).called(1);
        verify(() => repo.list()).called(1);
      },
    );
  });
}
