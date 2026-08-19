package repository

import (
	"context"

	"github.com/streampulse/backend/internal/domain/entity"
)

// AdminUserRepository is the user-store view the admin area needs, on top of
// the basic CRUD in UserRepository. The pgx *persistence.UserRepository
// satisfies both, so no separate adapter is required.
type AdminUserRepository interface {
	ListUsers(ctx context.Context, offset, limit int) ([]entity.User, error)
	CountUsersByRole(ctx context.Context) (map[entity.Role]int, error)
	FindByID(ctx context.Context, id string) (*entity.User, error)
	Update(ctx context.Context, u *entity.User) error
}
