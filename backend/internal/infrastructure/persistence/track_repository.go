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

const trackColumns = `t.id, t.title, t.artist, t.storage_key, t.filename,
	t.content_type, t.size_bytes, t.uploader_id, COALESCE(u.username, ''),
	t.created_at, t.updated_at`

type TrackRepository struct {
	db *sql.DB
}

func NewTrackRepository(db *sql.DB) *TrackRepository {
	return &TrackRepository{db: db}
}

var _ repository.TrackRepository = (*TrackRepository)(nil)

func (r *TrackRepository) Create(ctx context.Context, t *entity.AudioTrack) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	const q = `
		INSERT INTO tracks (id, title, artist, storage_key, filename, content_type, size_bytes, uploader_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`
	err := r.db.QueryRowContext(ctx, q,
		t.ID, t.Title, t.Artist, t.StorageKey, t.Filename, t.ContentType, t.SizeBytes, t.UploaderID,
	).Scan(&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgUniqueViolation:
				return repository.ErrConflict
			case pgForeignKeyViolation:
				return repository.ErrNotFound
			}
		}
		return fmt.Errorf("insert track: %w", err)
	}
	return nil
}

func (r *TrackRepository) FindByID(ctx context.Context, id string) (*entity.AudioTrack, error) {
	q := `SELECT ` + trackColumns + `
		FROM tracks t LEFT JOIN users u ON u.id = t.uploader_id
		WHERE t.id = $1`
	return scanTrack(r.db.QueryRowContext(ctx, q, id))
}

func (r *TrackRepository) List(ctx context.Context, limit, offset int) ([]entity.AudioTrack, error) {
	q := `SELECT ` + trackColumns + `
		FROM tracks t LEFT JOIN users u ON u.id = t.uploader_id
		ORDER BY t.created_at DESC
		LIMIT $1 OFFSET $2`
	return r.query(ctx, q, limit, offset)
}

func (r *TrackRepository) ListByUploader(ctx context.Context, uploaderID string) ([]entity.AudioTrack, error) {
	q := `SELECT ` + trackColumns + `
		FROM tracks t LEFT JOIN users u ON u.id = t.uploader_id
		WHERE t.uploader_id = $1
		ORDER BY t.created_at DESC`
	return r.query(ctx, q, uploaderID)
}

func (r *TrackRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tracks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete track: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *TrackRepository) query(ctx context.Context, q string, args ...any) ([]entity.AudioTrack, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query tracks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tracks := make([]entity.AudioTrack, 0)
	for rows.Next() {
		var t entity.AudioTrack
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.StorageKey, &t.Filename,
			&t.ContentType, &t.SizeBytes, &t.UploaderID, &t.UploaderUsername,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		tracks = append(tracks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tracks: %w", err)
	}
	return tracks, nil
}

func scanTrack(row *sql.Row) (*entity.AudioTrack, error) {
	var t entity.AudioTrack
	err := row.Scan(&t.ID, &t.Title, &t.Artist, &t.StorageKey, &t.Filename,
		&t.ContentType, &t.SizeBytes, &t.UploaderID, &t.UploaderUsername,
		&t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan track: %w", err)
	}
	return &t, nil
}
