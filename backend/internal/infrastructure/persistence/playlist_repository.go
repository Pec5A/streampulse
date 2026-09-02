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

type PlaylistRepository struct {
	db *sql.DB
}

func NewPlaylistRepository(db *sql.DB) *PlaylistRepository {
	return &PlaylistRepository{db: db}
}

var _ repository.PlaylistRepository = (*PlaylistRepository)(nil)

func (r *PlaylistRepository) Create(ctx context.Context, p *entity.Playlist) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	const q = `
		INSERT INTO playlists (id, owner_id, name, description, is_public)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`
	err := r.db.QueryRowContext(ctx, q, p.ID, p.OwnerID, p.Name, p.Description, p.IsPublic).
		Scan(&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert playlist: %w", err)
	}
	return nil
}

func (r *PlaylistRepository) FindByID(ctx context.Context, id string) (*entity.Playlist, error) {
	const q = `SELECT id, owner_id, name, description, is_public, created_at, updated_at
		FROM playlists WHERE id = $1`
	var p entity.Playlist
	err := r.db.QueryRowContext(ctx, q, id).
		Scan(&p.ID, &p.OwnerID, &p.Name, &p.Description, &p.IsPublic, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find playlist: %w", err)
	}
	return &p, nil
}

func (r *PlaylistRepository) ListByOwner(ctx context.Context, ownerID string) ([]entity.Playlist, error) {
	const q = `SELECT id, owner_id, name, description, is_public, created_at, updated_at
		FROM playlists WHERE owner_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list playlists: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []entity.Playlist
	for rows.Next() {
		var p entity.Playlist
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Description, &p.IsPublic, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan playlist: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PlaylistRepository) Update(ctx context.Context, p *entity.Playlist) error {
	const q = `UPDATE playlists SET name = $2, description = $3, is_public = $4, updated_at = now()
		WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, p.ID, p.Name, p.Description, p.IsPublic)
	if err != nil {
		return fmt.Errorf("update playlist: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *PlaylistRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM playlists WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete playlist: %w", err)
	}
	return checkRowsAffected(res)
}

func (r *PlaylistRepository) ListTracks(ctx context.Context, playlistID string) ([]entity.Track, error) {
	const q = `SELECT id, playlist_id, title, artist, duration_seconds, source_url, position, created_at
		FROM playlist_tracks WHERE playlist_id = $1 ORDER BY position`
	rows, err := r.db.QueryContext(ctx, q, playlistID)
	if err != nil {
		return nil, fmt.Errorf("list tracks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []entity.Track
	for rows.Next() {
		var t entity.Track
		if err := rows.Scan(&t.ID, &t.PlaylistID, &t.Title, &t.Artist, &t.DurationSeconds, &t.SourceURL, &t.Position, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AddTrack appends the track at the next free position, computed in the same
// statement. If two concurrent appends pick the same position, the deferred
// unique(playlist_id, position) constraint rejects the loser at commit, which
// we surface as repository.ErrConflict.
func (r *PlaylistRepository) AddTrack(ctx context.Context, t *entity.Track) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	const q = `
		INSERT INTO playlist_tracks (id, playlist_id, title, artist, duration_seconds, source_url, position)
		VALUES ($1, $2, $3, $4, $5, $6,
			COALESCE((SELECT MAX(position) + 1 FROM playlist_tracks WHERE playlist_id = $2), 0))
		RETURNING position, created_at`
	err := r.db.QueryRowContext(ctx, q, t.ID, t.PlaylistID, t.Title, t.Artist, t.DurationSeconds, t.SourceURL).
		Scan(&t.Position, &t.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return repository.ErrConflict
		}
		return fmt.Errorf("insert track: %w", err)
	}
	return nil
}

// RemoveTrack deletes the track and closes the positional gap in one
// transaction, so ListTracks never observes a hole.
func (r *PlaylistRepository) RemoveTrack(ctx context.Context, playlistID, trackID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var pos int
	err = tx.QueryRowContext(ctx,
		`DELETE FROM playlist_tracks WHERE playlist_id = $1 AND id = $2 RETURNING position`,
		playlistID, trackID).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete track: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE playlist_tracks SET position = position - 1 WHERE playlist_id = $1 AND position > $2`,
		playlistID, pos); err != nil {
		return fmt.Errorf("compact positions: %w", err)
	}
	return tx.Commit()
}

// ReorderTracks rewrites every listed track's position to its index, in one
// transaction. The unique(playlist_id, position) constraint is DEFERRABLE
// INITIALLY DEFERRED, so the intermediate duplicate positions produced during
// a row-by-row rewrite are tolerated and only the final arrangement is checked
// at COMMIT. A listed id that isn't in the playlist yields ErrNotFound (and
// rolls the whole reorder back).
func (r *PlaylistRepository) ReorderTracks(ctx context.Context, playlistID string, orderedTrackIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const q = `UPDATE playlist_tracks SET position = $1 WHERE playlist_id = $2 AND id = $3`
	for i, id := range orderedTrackIDs {
		res, err := tx.ExecContext(ctx, q, i, playlistID, id)
		if err != nil {
			return fmt.Errorf("reorder position %d: %w", i, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if n == 0 {
			return repository.ErrNotFound
		}
	}
	return tx.Commit()
}
