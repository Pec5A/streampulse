part of 'admin_bloc.dart';

sealed class AdminEvent extends Equatable {
  const AdminEvent();
  @override
  List<Object?> get props => [];
}

/// Load (or reload) stats and the user list.
final class AdminRequested extends AdminEvent {
  const AdminRequested();
}

/// Change a user's role, then reload.
final class AdminRoleChanged extends AdminEvent {
  const AdminRoleChanged({required this.userId, required this.role});
  final String userId;
  final String role;
  @override
  List<Object?> get props => [userId, role];
}
