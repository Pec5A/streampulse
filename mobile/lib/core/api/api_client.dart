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

  Future<Map<String, dynamic>> get(String path, {String? token}) async =>
      _decodeMap(await _send('GET', path, token: token));

  Future<List<dynamic>> getList(String path, {String? token}) async =>
      _decodeList(await _send('GET', path, token: token));

  Future<Map<String, dynamic>> post(String path, Map<String, dynamic> body, {String? token}) async =>
      _decodeMap(await _send('POST', path, body: body, token: token));

  Future<Map<String, dynamic>> patch(String path, Map<String, dynamic> body, {String? token}) async =>
      _decodeMap(await _send('PATCH', path, body: body, token: token));

  Future<Map<String, dynamic>> put(String path, Map<String, dynamic> body, {String? token}) async =>
      _decodeMap(await _send('PUT', path, body: body, token: token));

  Future<void> delete(String path, {String? token}) async =>
      _ensureSuccess(await _send('DELETE', path, token: token));

  Future<http.Response> _send(String method, String path, {Map<String, dynamic>? body, String? token}) {
    final uri = Uri.parse('$baseUrl$path');
    final headers = {
      'Content-Type': 'application/json',
      if (token != null) 'Authorization': 'Bearer $token',
    };
    final encoded = body == null ? null : jsonEncode(body);
    switch (method) {
      case 'GET':
        return _client.get(uri, headers: headers);
      case 'POST':
        return _client.post(uri, headers: headers, body: encoded);
      case 'PATCH':
        return _client.patch(uri, headers: headers, body: encoded);
      case 'PUT':
        return _client.put(uri, headers: headers, body: encoded);
      case 'DELETE':
        return _client.delete(uri, headers: headers);
      default:
        throw ArgumentError('unsupported method: $method');
    }
  }

  Map<String, dynamic> _decodeMap(http.Response res) {
    _ensureSuccess(res);
    return res.body.isEmpty ? <String, dynamic>{} : jsonDecode(res.body) as Map<String, dynamic>;
  }

  List<dynamic> _decodeList(http.Response res) {
    _ensureSuccess(res);
    return res.body.isEmpty ? <dynamic>[] : jsonDecode(res.body) as List<dynamic>;
  }

  void _ensureSuccess(http.Response res) {
    if (res.statusCode >= 200 && res.statusCode < 300) return;
    String? message;
    if (res.body.isNotEmpty) {
      final decoded = jsonDecode(res.body);
      if (decoded is Map<String, dynamic>) {
        message = decoded['error'] as String?;
      }
    }
    throw ApiException(res.statusCode, message ?? 'unknown error');
  }
}
