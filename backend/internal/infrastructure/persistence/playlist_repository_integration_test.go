//go:build integration

// Integration test for the pgx PlaylistRepository. Behind the `integration`
// tag because it needs a real Postgres; CI provides one and runs it. Locally:
//
//	DATABASE_URL=postgres://user:pass@localhost:5432/streampulse?sslmode=disable \
//	  go test -tags integration ./internal/infrastructure/persistence/
//
// The schema is applied by openMigratedDB — an empty database is enough.
package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

func TestPlaylistRepository_ReorderTracks_Integration(t *testing.T) {
	db := openMigratedDB(t)
	ctx := context.Background()

	// A user to own the playlist (owner_id has a FK to users).
	var ownerID string
	suffix := uuid.NewString()[:8]
	err := db.QueryRowContext(ctx,
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

func TestPlaylistRepository_CreateFindUpdateDelete_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPlaylistRepository(db)
	ctx := context.Background()
	owner := seedUser(t, db, entity.RoleUser)

	p := &entity.Playlist{OwnerID: owner.ID, Name: "Nuit blanche", Description: "jazz"}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if p.ID == "" || p.CreatedAt.IsZero() {
		t.Fatalf("Create() did not read back the generated id/timestamps: %+v", p)
	}

	got, err := repo.FindByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.Name != "Nuit blanche" || got.OwnerID != owner.ID || got.IsPublic {
		t.Errorf("FindByID() = %+v, want the created values and is_public false by default", got)
	}

	got.Name = "Nuit blanche v2"
	got.IsPublic = true
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	after, err := repo.FindByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("FindByID() after Update() error = %v", err)
	}
	if after.Name != "Nuit blanche v2" || !after.IsPublic {
		t.Errorf("after Update() = %+v, want the new name and is_public true", after)
	}

	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := repo.FindByID(ctx, p.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("FindByID() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestPlaylistRepository_MissingRowsAreNotFound_Integration(t *testing.T) {
	repo := NewPlaylistRepository(openMigratedDB(t))
	ctx := context.Background()
	const ghost = "00000000-0000-0000-0000-000000000000"

	if _, err := repo.FindByID(ctx, ghost); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("FindByID(missing) error = %v, want ErrNotFound", err)
	}
	if err := repo.Update(ctx, &entity.Playlist{ID: ghost, Name: "x"}); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Update(missing) error = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, ghost); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Delete(missing) error = %v, want ErrNotFound", err)
	}
}

func TestPlaylistRepository_ListByOwnerIsScopedToTheOwner_Integration(t *testing.T) {
	// The isolation between two users is enforced by the WHERE clause, so a
	// fake repository proves nothing here: only real SQL can be wrong.
	db := openMigratedDB(t)
	repo := NewPlaylistRepository(db)
	ctx := context.Background()

	mine := seedUser(t, db, entity.RoleUser)
	theirs := seedUser(t, db, entity.RoleUser)

	for _, name := range []string{"a", "b"} {
		if err := repo.Create(ctx, &entity.Playlist{OwnerID: mine.ID, Name: name}); err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
	}
	if err := repo.Create(ctx, &entity.Playlist{OwnerID: theirs.ID, Name: "not mine"}); err != nil {
		t.Fatalf("Create(other owner) error = %v", err)
	}

	got, err := repo.ListByOwner(ctx, mine.ID)
	if err != nil {
		t.Fatalf("ListByOwner() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByOwner() returned %d playlists, want exactly the 2 owned", len(got))
	}
	for _, p := range got {
		if p.OwnerID != mine.ID {
			t.Errorf("ListByOwner() leaked a playlist owned by %q", p.OwnerID)
		}
	}
}

func TestPlaylistRepository_AddListRemoveTracks_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewPlaylistRepository(db)
	ctx := context.Background()
	owner := seedUser(t, db, entity.RoleUser)

	p := &entity.Playlist{OwnerID: owner.ID, Name: "File d'attente"}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	titles := []string{"un", "deux", "trois"}
	ids := make([]string, 0, len(titles))
	for _, title := range titles {
		tr := &entity.Track{PlaylistID: p.ID, Title: title, Artist: "artiste", DurationSeconds: 120}
		if err := repo.AddTrack(ctx, tr); err != nil {
			t.Fatalf("AddTrack(%s) error = %v", title, err)
		}
		ids = append(ids, tr.ID)
	}

	tracks, err := repo.ListTracks(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListTracks() error = %v", err)
	}
	if len(tracks) != 3 {
		t.Fatalf("ListTracks() returned %d tracks, want 3", len(tracks))
	}
	for i, tr := range tracks {
		if tr.Title != titles[i] {
			t.Errorf("track %d = %q, want %q — ListTracks must order by position", i, tr.Title, titles[i])
		}
		// Positions are 0-based: AddTrack falls back to 0 on an empty
		// playlist (COALESCE(MAX(position)+1, 0)).
		if tr.Position != i {
			t.Errorf("track %q position = %d, want %d", tr.Title, tr.Position, i)
		}
	}

	// Removing the middle one must leave 1,2 — not 1,3. A hole in the
	// positions would make the next append collide with an existing one.
	if err := repo.RemoveTrack(ctx, p.ID, ids[1]); err != nil {
		t.Fatalf("RemoveTrack() error = %v", err)
	}
	tracks, err = repo.ListTracks(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListTracks() after RemoveTrack() error = %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("ListTracks() returned %d tracks after removal, want 2", len(tracks))
	}
	for i, tr := range tracks {
		if tr.Position != i {
			t.Errorf("after removal, %q is at position %d, want %d (positions must be compacted)", tr.Title, tr.Position, i)
		}
	}

	if err := repo.RemoveTrack(ctx, p.ID, ids[1]); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("RemoveTrack(already removed) error = %v, want ErrNotFound", err)
	}
}

func TestPlaylistRepository_DeletingTheOwnerCascades_Integration(t *testing.T) {
	// This is the cascade the RGPD erasure endpoint relies on: Y2 deletes the
	// user row and nothing else, so if the FK ever loses ON DELETE CASCADE the
	// account would be gone while its playlists survive — a silent breach of
	// the erasure guarantee, invisible to any test using a fake repository.
	db := openMigratedDB(t)
	repo := NewPlaylistRepository(db)
	ctx := context.Background()

	owner := seedUser(t, db, entity.RoleUser)
	p := &entity.Playlist{OwnerID: owner.ID, Name: "a effacer"}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repo.AddTrack(ctx, &entity.Track{PlaylistID: p.ID, Title: "piste"}); err != nil {
		t.Fatalf("AddTrack() error = %v", err)
	}

	if err := NewUserRepository(db).Delete(ctx, owner.ID); err != nil {
		t.Fatalf("Delete(user) error = %v", err)
	}

	if _, err := repo.FindByID(ctx, p.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("the playlist survived its owner's deletion (err = %v) — RGPD erasure is incomplete", err)
	}
	var remaining int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM playlist_tracks WHERE playlist_id = $1`, p.ID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining tracks: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d tracks survived the cascade, want 0", remaining)
	}
}
