package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

// ErrInvalidRole is returned when a role change targets an unknown role.
var ErrInvalidRole = errors.New("invalid role")

// Stats is a snapshot of user counts for the admin dashboard. It is scoped to
// the users table (the only one this slice owns) and can grow as other
// features' tables land.
type Stats struct {
	TotalUsers   int
	RegularUsers int
	Broadcasters int
	Admins       int
}

type AdminUseCase struct {
	users repository.AdminUserRepository
}

func NewAdminUseCase(users repository.AdminUserRepository) *AdminUseCase {
	return &AdminUseCase{users: users}
}

func (uc *AdminUseCase) ListUsers(ctx context.Context, offset, limit int) ([]entity.User, error) {
	return uc.users.ListUsers(ctx, offset, limit)
}

func (uc *AdminUseCase) Stats(ctx context.Context) (Stats, error) {
	counts, err := uc.users.CountUsersByRole(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("count users: %w", err)
	}
	s := Stats{
		RegularUsers: counts[entity.RoleUser],
		Broadcasters: counts[entity.RoleBroadcaster],
		Admins:       counts[entity.RoleAdmin],
	}
	s.TotalUsers = s.RegularUsers + s.Broadcasters + s.Admins
	return s, nil
}

// UpdateRole changes a user's role after validating it. Returns ErrInvalidRole
// for an unknown role and repository.ErrNotFound when the user is missing.
func (uc *AdminUseCase) UpdateRole(ctx context.Context, userID, role string) (*entity.User, error) {
	r := entity.Role(role)
	if !isValidRole(r) {
		return nil, ErrInvalidRole
	}
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u.Role = r
	if err := uc.users.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("update role: %w", err)
	}
	return u, nil
}

func isValidRole(r entity.Role) bool {
	switch r {
	case entity.RoleUser, entity.RoleBroadcaster, entity.RoleAdmin:
		return true
	default:
		return false
	}
}
