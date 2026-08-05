import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import 'core/api/api_client.dart';
import 'core/storage/secure_storage.dart';
import 'features/admin/bloc/admin_bloc.dart';
import 'features/admin/repositories/admin_repository.dart';
import 'features/admin/screens/admin_screen.dart';
import 'features/auth/bloc/auth_bloc.dart';
import 'features/auth/repository/auth_repository.dart';
import 'features/auth/screens/login_screen.dart';
import 'features/auth/user_model.dart';
import 'features/playlists/bloc/playlists_bloc.dart';
import 'features/playlists/repositories/playlist_repository.dart';
import 'features/playlists/screens/playlists_screen.dart';

void main() {
  const apiUrl = String.fromEnvironment('API_URL', defaultValue: 'http://localhost:8080');
  final apiClient = ApiClient(baseUrl: apiUrl);
  final storage = SecureStorage();
  final authRepository = AuthRepository(apiClient: apiClient, storage: storage);
  final adminRepository = AdminRepository(apiClient: apiClient, storage: storage);
  final playlistRepository = PlaylistRepository(apiClient: apiClient, storage: storage);

  runApp(StreamPulseApp(
    authRepository: authRepository,
    adminRepository: adminRepository,
    playlistRepository: playlistRepository,
  ));
}

class StreamPulseApp extends StatelessWidget {
  const StreamPulseApp({
    super.key,
    required this.authRepository,
    required this.adminRepository,
    required this.playlistRepository,
  });

  final AuthRepository authRepository;
  final AdminRepository adminRepository;
  final PlaylistRepository playlistRepository;

  @override
  Widget build(BuildContext context) {
    return MultiRepositoryProvider(
      providers: [
        RepositoryProvider.value(value: adminRepository),
        RepositoryProvider.value(value: playlistRepository),
      ],
      child: BlocProvider(
        create: (_) => AuthBloc(repository: authRepository),
        child: MaterialApp(
          title: 'StreamPulse',
          theme: _appTheme(),
          home: BlocListener<AuthBloc, AuthState>(
            listener: (context, state) {
              if (state is AuthAuthenticated) {
                Navigator.of(context).pushReplacement(
                  MaterialPageRoute(builder: (_) => HomeScreen(user: state.user)),
                );
              }
            },
            child: const LoginScreen(),
          ),
        ),
      ),
    );
  }
}

/// App theme with accessibility-friendly defaults: Material 3 colours (good
/// default contrast) and full 48dp touch targets for easier interaction.
ThemeData _appTheme() {
  return ThemeData(
    useMaterial3: true,
    colorScheme: ColorScheme.fromSeed(seedColor: Colors.deepPurple),
    materialTapTargetSize: MaterialTapTargetSize.padded,
  );
}

/// Landing screen after login: playlists for everyone, admin console for admins.
class HomeScreen extends StatelessWidget {
  const HomeScreen({super.key, required this.user});
  final UserModel user;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('StreamPulse')),
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text('Bienvenue, ${user.username}'),
            const SizedBox(height: 16),
            FilledButton.icon(
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => BlocProvider(
                    create: (ctx) =>
                        PlaylistsBloc(repository: ctx.read<PlaylistRepository>())..add(const PlaylistsRequested()),
                    child: const PlaylistsScreen(),
                  ),
                ),
              ),
              icon: const Icon(Icons.queue_music),
              label: const Text('Mes playlists'),
            ),
            if (user.role == 'admin') ...[
              const SizedBox(height: 12),
              FilledButton.icon(
                onPressed: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => BlocProvider(
                      create: (ctx) =>
                          AdminBloc(repository: ctx.read<AdminRepository>())..add(const AdminRequested()),
                      child: const AdminScreen(),
                    ),
                  ),
                ),
                icon: const Icon(Icons.admin_panel_settings),
                label: const Text('Administration'),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
