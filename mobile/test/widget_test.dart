import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:streampulse/core/api/api_client.dart';
import 'package:streampulse/core/storage/secure_storage.dart';
import 'package:streampulse/features/admin/repositories/admin_repository.dart';
import 'package:streampulse/features/auth/repository/auth_repository.dart';
import 'package:streampulse/main.dart';

void main() {
  testWidgets('App boots to the login screen', (WidgetTester tester) async {
    final apiClient = ApiClient(baseUrl: 'http://localhost:8080');
    final storage = SecureStorage();

    await tester.pumpWidget(StreamPulseApp(
      authRepository: AuthRepository(apiClient: apiClient, storage: storage),
      adminRepository: AdminRepository(apiClient: apiClient, storage: storage),
    ));

    expect(find.text('Connexion'), findsOneWidget);
    expect(find.widgetWithText(TextFormField, 'Email'), findsOneWidget);
    expect(find.text('Créer un compte'), findsOneWidget);
  });
}
