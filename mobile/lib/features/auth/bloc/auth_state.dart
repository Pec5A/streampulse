part of 'auth_bloc.dart';

sealed class AuthState extends Equatable {
  const AuthState();
  @override
  List<Object?> get props => [];
}

final class AuthInitial extends AuthState {
  const AuthInitial();
}

final class AuthLoading extends AuthState {
  const AuthLoading();
}

final class AuthAuthenticated extends AuthState {
  const AuthAuthenticated({required this.user});
  final UserModel user;
  @override
  List<Object?> get props => [user.id];
}

final class AuthUnauthenticated extends AuthState {
  const AuthUnauthenticated();
}

final class AuthFailure extends AuthState {
  const AuthFailure({required this.message});
  final String message;
  @override
  List<Object?> get props => [message];
}
