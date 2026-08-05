part of 'auth_bloc.dart';

sealed class AuthEvent extends Equatable {
  const AuthEvent();
  @override
  List<Object?> get props => [];
}

final class AuthRegisterRequested extends AuthEvent {
  const AuthRegisterRequested({required this.email, required this.username, required this.password});
  final String email;
  final String username;
  final String password;
  @override
  List<Object?> get props => [email, username, password];
}

final class AuthLoginRequested extends AuthEvent {
  const AuthLoginRequested({required this.email, required this.password});
  final String email;
  final String password;
  @override
  List<Object?> get props => [email, password];
}

final class AuthLogoutRequested extends AuthEvent {
  const AuthLogoutRequested();
}
