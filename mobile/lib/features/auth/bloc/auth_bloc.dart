import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../core/api/api_client.dart';
import '../repository/auth_repository.dart';
import '../user_model.dart';

part 'auth_event.dart';
part 'auth_state.dart';

class AuthBloc extends Bloc<AuthEvent, AuthState> {
  AuthBloc({required AuthRepository repository})
      : _repository = repository,
        super(const AuthInitial()) {
    on<AuthRegisterRequested>(_onRegister);
    on<AuthLoginRequested>(_onLogin);
    on<AuthLogoutRequested>(_onLogout);
  }

  final AuthRepository _repository;

  Future<void> _onRegister(AuthRegisterRequested event, Emitter<AuthState> emit) async {
    emit(const AuthLoading());
    try {
      final result = await _repository.register(event.email, event.username, event.password);
      emit(AuthAuthenticated(user: result.user));
    } on ApiException catch (e) {
      emit(AuthFailure(message: e.message));
    }
  }

  Future<void> _onLogin(AuthLoginRequested event, Emitter<AuthState> emit) async {
    emit(const AuthLoading());
    try {
      final result = await _repository.login(event.email, event.password);
      emit(AuthAuthenticated(user: result.user));
    } on ApiException catch (e) {
      emit(AuthFailure(message: e.message));
    }
  }

  Future<void> _onLogout(AuthLogoutRequested event, Emitter<AuthState> emit) async {
    await _repository.logout();
    emit(const AuthUnauthenticated());
  }
}
