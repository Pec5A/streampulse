import 'package:audio_service/audio_service.dart';
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
import 'features/player/audio/audio_engine.dart';
import 'features/player/audio/just_audio_engine.dart';
import 'features/player/audio/stream_audio_handler.dart';
import 'features/broadcaster/repositories/broadcaster_repository.dart';
import 'features/broadcaster/screens/broadcaster_screen.dart';
import 'features/player/repositories/stream_repository.dart';
import 'features/player/screens/live_streams_screen.dart';
import 'features/playlists/bloc/playlists_bloc.dart';
import 'features/playlists/repositories/playlist_repository.dart';
import 'features/playlists/screens/playlists_screen.dart';

const apiUrl = String.fromEnvironment('API_URL', defaultValue: 'http://localhost:8080');

Future<void> main() async {
  // AudioService.init touches platform channels, so the binding has to exist
  // first.
  WidgetsFlutterBinding.ensureInitialized();

  final apiClient = ApiClient(baseUrl: apiUrl);
  final storage = SecureStorage();
  final authRepository = AuthRepository(apiClient: apiClient, storage: storage);
  final adminRepository = AdminRepository(apiClient: apiClient, storage: storage);
  final playlistRepository = PlaylistRepository(apiClient: apiClient, storage: storage);
  final streamRepository = StreamRepository(baseUrl: apiUrl);
  final broadcasterRepository = BroadcasterRepository(baseUrl: apiUrl);

  // One handler for the whole app: it owns the audio session and keeps
  // playback alive in the background, with controls on the lock screen.
  final engine = JustAudioEngine();
  await engine.configure();
  final audioHandler = await AudioService.init(
    builder: () => StreamAudioHandler(engine),
    config: const AudioServiceConfig(
      androidNotificationChannelId: 'com.pec5a.streampulse.audio',
      androidNotificationChannelName: 'Lecture StreamPulse',
      // Keeps the foreground service (and therefore the audio) alive when
      // the app leaves the foreground.
      androidNotificationOngoing: true,
      androidStopForegroundOnPause: true,
    ),
  );

  runApp(StreamPulseApp(
    authRepository: authRepository,
    adminRepository: adminRepository,
    playlistRepository: playlistRepository,
    streamRepository: streamRepository,
    broadcasterRepository: broadcasterRepository,
    audioEngine: audioHandler,
  ));
}

class StreamPulseApp extends StatelessWidget {
  const StreamPulseApp({
    super.key,
    required this.authRepository,
    required this.adminRepository,
    required this.playlistRepository,
    this.streamRepository,
    this.broadcasterRepository,
    this.audioEngine,
  });

  final AuthRepository authRepository;
  final AdminRepository adminRepository;
  final PlaylistRepository playlistRepository;

  /// Optional so existing widget tests can boot the app without wiring the
  /// streaming stack; falls back to a repository pointed at [apiUrl].
  final StreamRepository? streamRepository;

  /// Optional for the same reason as [streamRepository].
  final BroadcasterRepository? broadcasterRepository;

  /// The shared background-capable engine. Null in tests, where each player
  /// screen builds its own.
  final AudioEngine? audioEngine;

  @override
  Widget build(BuildContext context) {
    final streams = streamRepository ?? StreamRepository(baseUrl: apiUrl);
    final broadcaster = broadcasterRepository ?? BroadcasterRepository(baseUrl: apiUrl);

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
                  MaterialPageRoute(
                    builder: (_) => HomeScreen(
                      user: state.user,
                      streamRepository: streams,
                      audioEngine: audioEngine,
                      broadcasterRepository: broadcaster,
                      authRepository: authRepository,
                    ),
                  ),
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

/// Landing screen after login: live streams and playlists for everyone, admin
/// console for admins.
class HomeScreen extends StatelessWidget {
  const HomeScreen({
    super.key,
    required this.user,
    required this.streamRepository,
    required this.broadcasterRepository,
    required this.authRepository,
    this.audioEngine,
  });

  final UserModel user;
  final StreamRepository streamRepository;
  final BroadcasterRepository broadcasterRepository;

  /// Needed to read the stored JWT when opening the broadcaster console.
  final AuthRepository authRepository;
  final AudioEngine? audioEngine;

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
                  builder: (_) => LiveStreamsScreen(
                    repository: streamRepository,
                    engine: audioEngine,
                  ),
                ),
              ),
              icon: const Icon(Icons.podcasts),
              label: const Text('En direct'),
            ),
            const SizedBox(height: 12),
            FilledButton.icon(
              onPressed: () => _openBroadcaster(context, broadcasterRepository, authRepository),
              icon: const Icon(Icons.mic),
              label: const Text('Diffuser'),
            ),
            const SizedBox(height: 12),
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

/// Reads the stored JWT, then opens the broadcaster console.
///
/// The token is read on demand rather than held in the widget tree: it lives
/// in secure storage, and keeping a copy around longer than a single call is
/// exactly the kind of thing that ends up in a crash log.
Future<void> _openBroadcaster(
  BuildContext context,
  BroadcasterRepository repository,
  AuthRepository auth,
) async {
  final token = await auth.readStoredToken();
  if (!context.mounted) return;

  if (token == null) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Session expirée, reconnecte-toi.')),
    );
    return;
  }

  await Navigator.of(context).push(
    MaterialPageRoute<void>(
      builder: (_) => BroadcasterScreen(repository: repository, token: token),
    ),
  );
}
