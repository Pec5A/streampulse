import 'package:equatable/equatable.dart';

/// A user row in the admin console (see backend UserResponse).
class AdminUser extends Equatable {
  const AdminUser({
    required this.id,
    required this.email,
    required this.username,
    required this.role,
  });

  factory AdminUser.fromJson(Map<String, dynamic> json) => AdminUser(
        id: json['id'] as String,
        email: (json['email'] as String?) ?? '',
        username: (json['username'] as String?) ?? '',
        role: (json['role'] as String?) ?? 'user',
      );

  final String id;
  final String email;
  final String username;
  final String role;

  @override
  List<Object?> get props => [id, email, username, role];
}

/// Platform snapshot for the admin dashboard (see backend StatsResponse).
class AdminStats extends Equatable {
  const AdminStats({
    required this.totalUsers,
    required this.totalRegular,
    required this.totalBroadcasters,
    required this.totalAdmins,
  });

  factory AdminStats.fromJson(Map<String, dynamic> json) => AdminStats(
        totalUsers: (json['total_users'] as num?)?.toInt() ?? 0,
        totalRegular: (json['total_regular'] as num?)?.toInt() ?? 0,
        totalBroadcasters: (json['total_broadcasters'] as num?)?.toInt() ?? 0,
        totalAdmins: (json['total_admins'] as num?)?.toInt() ?? 0,
      );

  final int totalUsers;
  final int totalRegular;
  final int totalBroadcasters;
  final int totalAdmins;

  @override
  List<Object?> get props => [totalUsers, totalRegular, totalBroadcasters, totalAdmins];
}
