import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../core/layout/responsive.dart';
import '../bloc/admin_bloc.dart';
import '../models/admin_models.dart';

/// Admin console: platform stats and user role management. Built with
/// accessibility in mind (Semantics labels, native ListTile semantics,
/// 48dp targets from the app theme) and a responsive, width-capped layout.
class AdminScreen extends StatelessWidget {
  const AdminScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 2,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Administration'),
          bottom: const TabBar(
            tabs: [
              Tab(icon: Icon(Icons.insights), text: 'Statistiques'),
              Tab(icon: Icon(Icons.people), text: 'Utilisateurs'),
            ],
          ),
        ),
        body: BlocConsumer<AdminBloc, AdminState>(
          listenWhen: (previous, current) => current is AdminFailure,
          listener: (context, state) {
            if (state is AdminFailure) {
              ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(state.message)));
            }
          },
          builder: (context, state) => switch (state) {
            AdminLoading() => const Center(child: CircularProgressIndicator()),
            AdminFailure(:final message) => _Retry(message: message),
            AdminLoaded(:final stats, :final users) => TabBarView(
                children: [
                  _StatsTab(stats: stats),
                  _UsersTab(users: users),
                ],
              ),
          },
        ),
      ),
    );
  }
}

class _StatsTab extends StatelessWidget {
  const _StatsTab({required this.stats});
  final AdminStats stats;

  @override
  Widget build(BuildContext context) {
    final cards = <({String label, int value, IconData icon})>[
      (label: 'Utilisateurs', value: stats.totalUsers, icon: Icons.people),
      (label: 'Auditeurs', value: stats.totalRegular, icon: Icons.person),
      (label: 'Diffuseurs', value: stats.totalBroadcasters, icon: Icons.mic),
      (label: 'Admins', value: stats.totalAdmins, icon: Icons.shield),
    ];
    return ContentWidth(
      child: GridView.builder(
        padding: const EdgeInsets.all(16),
        gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
          maxCrossAxisExtent: 240,
          childAspectRatio: 1.3,
          mainAxisSpacing: 12,
          crossAxisSpacing: 12,
        ),
        itemCount: cards.length,
        itemBuilder: (context, i) {
          final c = cards[i];
          // Merge the icon/number/label into one node read as "42 Utilisateurs".
          return Semantics(
            label: '${c.value} ${c.label}',
            child: ExcludeSemantics(
              child: Card(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Icon(c.icon, size: 32),
                      const SizedBox(height: 8),
                      Text('${c.value}', style: Theme.of(context).textTheme.headlineMedium),
                      Text(c.label, textAlign: TextAlign.center),
                    ],
                  ),
                ),
              ),
            ),
          );
        },
      ),
    );
  }
}

class _UsersTab extends StatelessWidget {
  const _UsersTab({required this.users});
  final List<AdminUser> users;

  @override
  Widget build(BuildContext context) {
    if (users.isEmpty) {
      return const Center(child: Text('Aucun utilisateur'));
    }
    return ContentWidth(
      child: ListView.separated(
        itemCount: users.length,
        separatorBuilder: (_, _) => const Divider(height: 1),
        itemBuilder: (context, i) {
          final u = users[i];
          return ListTile(
            leading: CircleAvatar(child: Text(u.username.isEmpty ? '?' : u.username[0].toUpperCase())),
            title: Text(u.username),
            subtitle: Text(u.email),
            trailing: Chip(label: Text(u.role)),
            onTap: () => _changeRole(context, u),
          );
        },
      ),
    );
  }

  Future<void> _changeRole(BuildContext context, AdminUser u) async {
    final bloc = context.read<AdminBloc>();
    final role = await showDialog<String>(
      context: context,
      builder: (dialogContext) => SimpleDialog(
        title: Text('Rôle de ${u.username}'),
        children: [
          for (final r in const ['user', 'broadcaster', 'admin'])
            SimpleDialogOption(
              onPressed: () => Navigator.of(dialogContext).pop(r),
              child: Semantics(
                selected: r == u.role,
                child: Row(
                  children: [
                    Icon(r == u.role ? Icons.radio_button_checked : Icons.radio_button_unchecked),
                    const SizedBox(width: 12),
                    Text(r),
                  ],
                ),
              ),
            ),
        ],
      ),
    );
    if (role != null && role != u.role) {
      bloc.add(AdminRoleChanged(userId: u.id, role: role));
    }
  }
}

class _Retry extends StatelessWidget {
  const _Retry({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(message),
          const SizedBox(height: 8),
          OutlinedButton(
            onPressed: () => context.read<AdminBloc>().add(const AdminRequested()),
            child: const Text('Réessayer'),
          ),
        ],
      ),
    );
  }
}
