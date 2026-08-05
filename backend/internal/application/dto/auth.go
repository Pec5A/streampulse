package dto

import "github.com/streampulse/backend/internal/domain/entity"

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type AuthResponse struct {
	User  UserResponse `json:"user"`
	Token string       `json:"token"`
}

func UserFrom(u *entity.User) UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Username: u.Username, Role: string(u.Role)}
}
