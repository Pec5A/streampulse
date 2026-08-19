part of 'admin_bloc.dart';

sealed class AdminState extends Equatable {
  const AdminState();
  @override
  List<Object?> get props => [];
}

final class AdminLoading extends AdminState {
  const AdminLoading();
}

final class AdminLoaded extends AdminState {
  const AdminLoaded({required this.stats, required this.users});
  final AdminStats stats;
  final List<AdminUser> users;
  @override
  List<Object?> get props => [stats, users];
}

final class AdminFailure extends AdminState {
  const AdminFailure(this.message);
  final String message;
  @override
  List<Object?> get props => [message];
}
