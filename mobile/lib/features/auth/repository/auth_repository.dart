import '../../../core/api/api_client.dart';
import '../../../core/storage/secure_storage.dart';
import '../user_model.dart';

class AuthResult {
  const AuthResult({required this.user, required this.token});
  final UserModel user;
  final String token;
}

class AuthRepository {
  AuthRepository({required ApiClient apiClient, required SecureStorage storage})
      : _api = apiClient,
        _storage = storage;

  final ApiClient _api;
  final SecureStorage _storage;

  Future<AuthResult> register(String email, String username, String password) async {
    final json = await _api.post('/api/v1/auth/register', {
      'email': email,
      'username': username,
      'password': password,
    });
    return _persist(json);
  }

  Future<AuthResult> login(String email, String password) async {
    final json = await _api.post('/api/v1/auth/login', {
      'email': email,
      'password': password,
    });
    return _persist(json);
  }

  Future<String?> readStoredToken() => _storage.readToken();

  Future<void> logout() => _storage.clearToken();

  Future<AuthResult> _persist(Map<String, dynamic> json) async {
    final token = json['token'] as String;
    await _storage.saveToken(token);
    return AuthResult(user: UserModel.fromJson(json['user'] as Map<String, dynamic>), token: token);
  }
}
