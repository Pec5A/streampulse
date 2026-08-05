class UserModel {
  const UserModel({
    required this.id,
    required this.email,
    required this.username,
    required this.role,
  });

  factory UserModel.fromJson(Map<String, dynamic> json) => UserModel(
        id: json['id'] as String,
        email: json['email'] as String,
        username: json['username'] as String,
        role: json['role'] as String,
      );

  final String id;
  final String email;
  final String username;
  final String role;
}
