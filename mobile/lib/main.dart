import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import 'core/api/api_client.dart';
import 'core/storage/secure_storage.dart';
import 'features/auth/bloc/auth_bloc.dart';
import 'features/auth/repository/auth_repository.dart';
import 'features/auth/screens/login_screen.dart';

void main() {
  const apiUrl = String.fromEnvironment('API_URL', defaultValue: 'http://localhost:8080');
  final apiClient = ApiClient(baseUrl: apiUrl);
  final storage = SecureStorage();
  final authRepository = AuthRepository(apiClient: apiClient, storage: storage);

  runApp(StreamPulseApp(authRepository: authRepository));
}

class StreamPulseApp extends StatelessWidget {
  const StreamPulseApp({super.key, required this.authRepository});

  final AuthRepository authRepository;

  @override
  Widget build(BuildContext context) {
    return BlocProvider(
      create: (_) => AuthBloc(repository: authRepository),
      child: MaterialApp(
        title: 'StreamPulse',
        theme: ThemeData(colorScheme: ColorScheme.fromSeed(seedColor: Colors.deepPurple)),
        home: BlocListener<AuthBloc, AuthState>(
          listener: (context, state) {
            if (state is AuthAuthenticated) {
              Navigator.of(context).pushReplacement(
                MaterialPageRoute(builder: (_) => _HomePlaceholder(username: state.user.username)),
              );
            }
          },
          child: const LoginScreen(),
        ),
      ),
    );
  }
}

/// Placeholder landing screen — replaced once the streams/playlists
/// features (other tickets) land.
class _HomePlaceholder extends StatelessWidget {
  const _HomePlaceholder({required this.username});
  final String username;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('StreamPulse')),
      body: Center(child: Text('Bienvenue, $username')),
    );
  }
}
