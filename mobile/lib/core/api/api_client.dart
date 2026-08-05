import 'dart:convert';
import 'package:http/http.dart' as http;

/// Thrown when the API returns a non-2xx status; carries the parsed error
/// message (if any) so callers/blocs can show something meaningful.
class ApiException implements Exception {
  ApiException(this.statusCode, this.message);
  final int statusCode;
  final String message;

  @override
  String toString() => 'ApiException($statusCode): $message';
}

/// Thin wrapper around [http.Client] — base URL + JSON encode/decode +
/// error mapping. Deliberately not a generated client (no OpenAPI codegen
/// yet): the surface is small enough that hand-writing it is clearer.
class ApiClient {
  ApiClient({required this.baseUrl, http.Client? client}) : _client = client ?? http.Client();

  final String baseUrl;
  final http.Client _client;

  Future<Map<String, dynamic>> post(
    String path,
    Map<String, dynamic> body, {
    String? token,
  }) async {
    final res = await _client.post(
      Uri.parse('$baseUrl$path'),
      headers: {
        'Content-Type': 'application/json',
        if (token != null) 'Authorization': 'Bearer $token',
      },
      body: jsonEncode(body),
    );
    return _decode(res);
  }

  Map<String, dynamic> _decode(http.Response res) {
    final decoded = res.body.isEmpty ? <String, dynamic>{} : jsonDecode(res.body) as Map<String, dynamic>;
    if (res.statusCode >= 200 && res.statusCode < 300) {
      return decoded;
    }
    throw ApiException(res.statusCode, decoded['error'] as String? ?? 'unknown error');
  }
}
