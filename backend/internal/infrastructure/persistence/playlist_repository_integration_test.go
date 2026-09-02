//go:build integration

// Integration test for the pgx PlaylistRepository. Excluded from the default
// build/CI (no database there); run it against a real Postgres with:
//
//	DATABASE_URL=postgres://user:pass@localhost:5432/streampulse?sslmode=disable \
//	  go test -tags integration ./internal/infrastructure/persistence/
//
// Requires migrations 0001 + 0002 to have been applied.
package persistence

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/streampulse/backend/internal/domain/entity"
)

func TestPlaylistRepository_ReorderTracks_Integration(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()

	// A user to own the playlist (owner_id has a FK to users).
	var ownerID string
	suffix := uuid.NewString()[:8]
	err = db.QueryRowContext(ctx,
		`INSERT INTO users (email, username, password, role) VALUES ($1, $2, $3, 'user') RETURNING id`,
		"owner-"+suffix+"@test.local", "owner-"+suffix, "not-a-real-hash").Scan(&ownerID)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, ownerID) })

	repo := NewPlaylistRepository(db)

	p := &entity.Playlist{OwnerID: ownerID, Name: "Integration"}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("create playlist: %v", err)
	}

	var ids []string
	for _, title := range []string{"A", "B", "C"} {
		tr := &entity.Track{PlaylistID: p.ID, Title: title}
		if err := repo.AddTrack(ctx, tr); err != nil {
			t.Fatalf("add track %s: %v", title, err)
		}
		ids = append(ids, tr.ID)
	}

	// Reverse the queue to C, B, A. Rewriting positions row-by-row produces
	// overlapping intermediate positions that only commit because the
	// unique(playlist_id, position) constraint is DEFERRABLE INITIALLY DEFERRED.
	reversed := []string{ids[2], ids[1], ids[0]}
	if err := repo.ReorderTracks(ctx, p.ID, reversed); err != nil {
		t.Fatalf("reorder: %v", err)
	}

	got, err := repo.ListTracks(ctx, p.ID)
	if err != nil {
		t.Fatalf("list tracks: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []string{"C", "B", "A"} {
		if got[i].Title != want || got[i].Position != i {
			t.Fatalf("track %d = %q@%d, want %s@%d", i, got[i].Title, got[i].Position, want, i)
		}
	}
}
