import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../core/api/api_client.dart';
import '../models/admin_models.dart';
import '../repositories/admin_repository.dart';

part 'admin_event.dart';
part 'admin_state.dart';

/// Drives the admin console: load stats + users, and change a user's role.
class AdminBloc extends Bloc<AdminEvent, AdminState> {
  AdminBloc({required AdminRepository repository})
      : _repository = repository,
        super(const AdminLoading()) {
    on<AdminRequested>(_onRequested);
    on<AdminRoleChanged>(_onRoleChanged);
  }

  final AdminRepository _repository;

  Future<void> _onRequested(AdminRequested event, Emitter<AdminState> emit) async {
    emit(const AdminLoading());
    await _load(emit);
  }

  Future<void> _onRoleChanged(AdminRoleChanged event, Emitter<AdminState> emit) async {
    try {
      await _repository.updateRole(event.userId, event.role);
    } on ApiException catch (e) {
      emit(AdminFailure(e.message));
      return;
    }
    await _load(emit);
  }

  Future<void> _load(Emitter<AdminState> emit) async {
    try {
      final stats = await _repository.stats();
      final users = await _repository.listUsers();
      emit(AdminLoaded(stats: stats, users: users));
    } on ApiException catch (e) {
      emit(AdminFailure(e.message));
    }
  }
}
