import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/features/auth/bloc/auth_bloc.dart';
import 'package:streampulse/features/auth/repository/auth_repository.dart';
import 'package:streampulse/features/auth/user_model.dart';

class MockAuthRepository extends Mock implements AuthRepository {}

void main() {
  late MockAuthRepository repository;

  setUp(() {
    repository = MockAuthRepository();
  });

  const user = UserModel(id: 'u1', email: 'a@b.com', username: 'alice', role: 'user');

  group('AuthBloc login', () {
    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthAuthenticated] on successful login',
      setUp: () {
        when(() => repository.login('a@b.com', 'hunter2222'))
            .thenAnswer((_) async => const AuthResult(user: user, token: 'jwt-token'));
      },
      build: () => AuthBloc(repository: repository),
      act: (bloc) => bloc.add(const AuthLoginRequested(email: 'a@b.com', password: 'hunter2222')),
      expect: () => [const AuthLoading(), const AuthAuthenticated(user: user)],
    );

    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthFailure] when the repository throws ApiException',
      setUp: () {
        when(() => repository.login('a@b.com', 'wrong'))
            .thenThrow(ApiException(401, 'invalid credentials'));
      },
      build: () => AuthBloc(repository: repository),
      act: (bloc) => bloc.add(const AuthLoginRequested(email: 'a@b.com', password: 'wrong')),
      expect: () => [const AuthLoading(), const AuthFailure(message: 'invalid credentials')],
    );
  });

  group('AuthBloc register', () {
    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthAuthenticated] on successful register',
      setUp: () {
        when(() => repository.register('a@b.com', 'alice', 'hunter2222'))
            .thenAnswer((_) async => const AuthResult(user: user, token: 'jwt-token'));
      },
      build: () => AuthBloc(repository: repository),
      act: (bloc) => bloc.add(
        const AuthRegisterRequested(email: 'a@b.com', username: 'alice', password: 'hunter2222'),
      ),
      expect: () => [const AuthLoading(), const AuthAuthenticated(user: user)],
    );

    blocTest<AuthBloc, AuthState>(
      'emits [AuthLoading, AuthFailure] when email is already taken',
      setUp: () {
        when(() => repository.register('dup@b.com', 'dup', 'hunter2222'))
            .thenThrow(ApiException(409, 'email already registered'));
      },
      build: () => AuthBloc(repository: repository),
      act: (bloc) => bloc.add(
        const AuthRegisterRequested(email: 'dup@b.com', username: 'dup', password: 'hunter2222'),
      ),
      expect: () => [const AuthLoading(), const AuthFailure(message: 'email already registered')],
    );
  });

  group('AuthBloc logout', () {
    blocTest<AuthBloc, AuthState>(
      'emits [AuthUnauthenticated] and clears stored token',
      setUp: () {
        when(() => repository.logout()).thenAnswer((_) async {});
      },
      build: () => AuthBloc(repository: repository),
      act: (bloc) => bloc.add(const AuthLogoutRequested()),
      expect: () => [const AuthUnauthenticated()],
      verify: (_) {
        verify(() => repository.logout()).called(1);
      },
    );
  });
}
