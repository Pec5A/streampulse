package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

const pgUniqueViolation = "23505"

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

var _ repository.UserRepository = (*UserRepository)(nil)

func (r *UserRepository) Create(ctx context.Context, u *entity.User) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	const q = `
		INSERT INTO users (id, email, username, password, role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`
	err := r.db.QueryRowContext(ctx, q, u.ID, u.Email, u.Username, u.Password, u.Role).
		Scan(&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return repository.ErrConflict
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*entity.User, error) {
	const q = `SELECT id, email, username, password, role, created_at, updated_at FROM users WHERE id = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, id))
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	const q = `SELECT id, email, username, password, role, created_at, updated_at FROM users WHERE email = $1`
	return r.scanOne(r.db.QueryRowContext(ctx, q, email))
}

func (r *UserRepository) Update(ctx context.Context, u *entity.User) error {
	const q = `
		UPDATE users SET email = $2, username = $3, role = $4, updated_at = now()
		WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, u.ID, u.Email, u.Username, u.Role)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *UserRepository) scanOne(row *sql.Row) (*entity.User, error) {
	var u entity.User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}

func checkRowsAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}
