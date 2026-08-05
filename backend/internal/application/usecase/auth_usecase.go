package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/auth"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type AuthUseCase struct {
	users  repository.UserRepository
	jwt    *auth.JWTManager
	hasher auth.PasswordHasher
}

func NewAuthUseCase(users repository.UserRepository, jwt *auth.JWTManager, hasher auth.PasswordHasher) *AuthUseCase {
	return &AuthUseCase{users: users, jwt: jwt, hasher: hasher}
}

// Register creates a new user account and returns it with a fresh JWT.
// Callers may receive repository.ErrConflict if the email or username is
// already taken.
func (uc *AuthUseCase) Register(ctx context.Context, email, username, password string) (*entity.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	username = strings.TrimSpace(username)

	if _, err := uc.users.FindByEmail(ctx, email); err == nil {
		return nil, "", repository.ErrConflict
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, "", fmt.Errorf("lookup email: %w", err)
	}

	hashed, err := uc.hasher.Hash(password)
	if err != nil {
		return nil, "", fmt.Errorf("hash password: %w", err)
	}

	user := &entity.User{
		Email:    email,
		Username: username,
		Password: hashed,
		Role:     entity.RoleUser,
	}
	if err := uc.users.Create(ctx, user); err != nil {
		return nil, "", fmt.Errorf("create user: %w", err)
	}

	token, err := uc.jwt.Generate(user.ID, string(user.Role))
	if err != nil {
		return nil, "", fmt.Errorf("generate token: %w", err)
	}
	return user, token, nil
}

func (uc *AuthUseCase) Login(ctx context.Context, email, password string) (*entity.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	user, err := uc.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", fmt.Errorf("find user: %w", err)
	}

	if !uc.hasher.Verify(password, user.Password) {
		return nil, "", ErrInvalidCredentials
	}

	token, err := uc.jwt.Generate(user.ID, string(user.Role))
	if err != nil {
		return nil, "", fmt.Errorf("generate token: %w", err)
	}
	return user, token, nil
}

// Refresh re-issues a JWT for an already-authenticated user (sliding
// window: using the app before expiry gets a fresh token, no re-login).
func (uc *AuthUseCase) Refresh(ctx context.Context, userID string) (*entity.User, string, error) {
	user, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	token, err := uc.jwt.Generate(user.ID, string(user.Role))
	if err != nil {
		return nil, "", fmt.Errorf("generate token: %w", err)
	}
	return user, token, nil
}
