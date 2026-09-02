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

const pgForeignKeyViolation = "23503"

// streamColumns is shared by every read so the scan order can never drift
// between queries.
const streamColumns = `s.id, s.title, s.description, s.broadcaster_id,
	COALESCE(u.username, ''), s.status, s.created_at, s.updated_at`

type StreamRepository struct {
	db *sql.DB
}

func NewStreamRepository(db *sql.DB) *StreamRepository {
	return &StreamRepository{db: db}
}

var _ repository.StreamRepository = (*StreamRepository)(nil)

func (r *StreamRepository) Create(ctx context.Context, s *entity.Stream) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == "" {
		s.Status = entity.StreamStatusOffline
	}
	const q = `
		INSERT INTO streams (id, title, description, broadcaster_id, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`
	err := r.db.QueryRowContext(ctx, q, s.ID, s.Title, s.Description, s.BroadcasterID, s.Status).
		Scan(&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgUniqueViolation:
				return repository.ErrConflict
			case pgForeignKeyViolation:
				// The broadcaster id does not reference a real user.
				return repository.ErrNotFound
			}
		}
		return fmt.Errorf("insert stream: %w", err)
	}
	return nil
}

func (r *StreamRepository) FindByID(ctx context.Context, id string) (*entity.Stream, error) {
	q := `SELECT ` + streamColumns + `
		FROM streams s LEFT JOIN users u ON u.id = s.broadcaster_id
		WHERE s.id = $1`
	return scanStream(r.db.QueryRowContext(ctx, q, id))
}

func (r *StreamRepository) List(ctx context.Context) ([]entity.Stream, error) {
	q := `SELECT ` + streamColumns + `
		FROM streams s LEFT JOIN users u ON u.id = s.broadcaster_id
		ORDER BY s.created_at DESC`
	return r.query(ctx, q)
}

func (r *StreamRepository) ListByStatus(ctx context.Context, status entity.StreamStatus) ([]entity.Stream, error) {
	q := `SELECT ` + streamColumns + `
		FROM streams s LEFT JOIN users u ON u.id = s.broadcaster_id
		WHERE s.status = $1
		ORDER BY s.created_at DESC`
	return r.query(ctx, q, status)
}

func (r *StreamRepository) UpdateStatus(ctx context.Context, id string, status entity.StreamStatus) error {
	const q = `UPDATE streams SET status = $2, updated_at = now() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id, status)
	if err != nil {
		return fmt.Errorf("update stream status: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *StreamRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM streams WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete stream: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *StreamRepository) query(ctx context.Context, q string, args ...any) ([]entity.Stream, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query streams: %w", err)
	}
	defer func() { _ = rows.Close() }()

	streams := make([]entity.Stream, 0)
	for rows.Next() {
		var s entity.Stream
		if err := rows.Scan(&s.ID, &s.Title, &s.Description, &s.BroadcasterID,
			&s.BroadcasterUsername, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan stream: %w", err)
		}
		streams = append(streams, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate streams: %w", err)
	}
	return streams, nil
}

func scanStream(row *sql.Row) (*entity.Stream, error) {
	var s entity.Stream
	err := row.Scan(&s.ID, &s.Title, &s.Description, &s.BroadcasterID,
		&s.BroadcasterUsername, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan stream: %w", err)
	}
	return &s, nil
}
