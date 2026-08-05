import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/features/admin/bloc/admin_bloc.dart';
import 'package:streampulse/features/admin/models/admin_models.dart';
import 'package:streampulse/features/admin/repositories/admin_repository.dart';

class MockAdminRepository extends Mock implements AdminRepository {}

void main() {
  late MockAdminRepository repo;
  setUp(() => repo = MockAdminRepository());

  const stats = AdminStats(totalUsers: 3, totalRegular: 1, totalBroadcasters: 1, totalAdmins: 1);
  const users = [
    AdminUser(id: '1', email: 'a@x', username: 'a', role: 'user'),
    AdminUser(id: '2', email: 'b@x', username: 'b', role: 'admin'),
  ];

  group('AdminBloc', () {
    blocTest<AdminBloc, AdminState>(
      'emits [Loading, Loaded] on AdminRequested',
      setUp: () {
        when(() => repo.stats()).thenAnswer((_) async => stats);
        when(() => repo.listUsers()).thenAnswer((_) async => users);
      },
      build: () => AdminBloc(repository: repo),
      act: (bloc) => bloc.add(const AdminRequested()),
      expect: () => const [AdminLoading(), AdminLoaded(stats: stats, users: users)],
    );

    blocTest<AdminBloc, AdminState>(
      'emits [Loading, Failure] when loading fails',
      setUp: () => when(() => repo.stats()).thenThrow(ApiException(403, 'admin only')),
      build: () => AdminBloc(repository: repo),
      act: (bloc) => bloc.add(const AdminRequested()),
      expect: () => const [AdminLoading(), AdminFailure('admin only')],
    );

    blocTest<AdminBloc, AdminState>(
      'changes a role then reloads',
      setUp: () {
        when(() => repo.updateRole('1', 'broadcaster')).thenAnswer(
          (_) async => const AdminUser(id: '1', email: 'a@x', username: 'a', role: 'broadcaster'),
        );
        when(() => repo.stats()).thenAnswer((_) async => stats);
        when(() => repo.listUsers()).thenAnswer((_) async => users);
      },
      build: () => AdminBloc(repository: repo),
      act: (bloc) => bloc.add(const AdminRoleChanged(userId: '1', role: 'broadcaster')),
      expect: () => const [AdminLoaded(stats: stats, users: users)],
      verify: (_) => verify(() => repo.updateRole('1', 'broadcaster')).called(1),
    );
  });
}
