package persistence

import (
	"context"
	"fmt"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

// The user repository also serves the admin read model (list + role counts).
// These methods live in their own file so the auth ticket's user_repository.go
// stays untouched.
var _ repository.AdminUserRepository = (*UserRepository)(nil)

// ListUsers returns users newest-first, paginated. limit is clamped to
// [1,200] (default 50 when out of range) and a negative offset is treated as 0.
func (r *UserRepository) ListUsers(ctx context.Context, offset, limit int) ([]entity.User, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	const q = `SELECT id, email, username, password, role, created_at, updated_at
		FROM users ORDER BY created_at DESC OFFSET $1 LIMIT $2`
	rows, err := r.db.QueryContext(ctx, q, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []entity.User
	for rows.Next() {
		var u entity.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsersByRole returns the number of users for each role present.
func (r *UserRepository) CountUsersByRole(ctx context.Context) (map[entity.Role]int, error) {
	const q = `SELECT role, COUNT(*) FROM users GROUP BY role`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("count users by role: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[entity.Role]int)
	for rows.Next() {
		var role string
		var n int
		if err := rows.Scan(&role, &n); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		out[entity.Role(role)] = n
	}
	return out, rows.Err()
}
