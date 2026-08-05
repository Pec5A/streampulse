import '../../../core/api/api_client.dart';
import '../../../core/storage/secure_storage.dart';
import '../models/admin_models.dart';

/// Talks to the admin API, attaching the stored JWT to every request.
class AdminRepository {
  AdminRepository({required ApiClient apiClient, required SecureStorage storage})
      : _api = apiClient,
        _storage = storage;

  final ApiClient _api;
  final SecureStorage _storage;

  static const _base = '/api/v1/admin';

  Future<AdminStats> stats() async {
    final json = await _api.get('$_base/stats', token: await _storage.readToken());
    return AdminStats.fromJson(json);
  }

  Future<List<AdminUser>> listUsers({int offset = 0, int limit = 100}) async {
    final json = await _api.getList(
      '$_base/users?offset=$offset&limit=$limit',
      token: await _storage.readToken(),
    );
    return json.map((e) => AdminUser.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<AdminUser> updateRole(String userId, String role) async {
    final json = await _api.patch(
      '$_base/users/$userId/role',
      {'role': role},
      token: await _storage.readToken(),
    );
    return AdminUser.fromJson(json);
  }
}
