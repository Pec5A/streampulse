//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

// These cover TrackRepository against a real Postgres, for the same reason
// the user and playlist ones exist: the fake repository used by the use-case
// tests agrees with whatever the fake was written to do, not with what the
// SQL actually does. Constraints, joins and cascades only exist here.

func seedTrack(t *testing.T, repo *TrackRepository, uploaderID, title string) *entity.AudioTrack {
	t.Helper()

	track := &entity.AudioTrack{
		Title:       title,
		Artist:      "KaysZ",
		StorageKey:  uuid.NewString() + ".mp3",
		Filename:    "nocturne.mp3",
		ContentType: "audio/mpeg",
		SizeBytes:   4096,
		UploaderID:  uploaderID,
	}
	if err := repo.Create(context.Background(), track); err != nil {
		t.Fatalf("seed track: %v", err)
	}
	return track
}

func TestTrackRepository_CreateThenFind_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewTrackRepository(db)
	user := seedUser(t, db, entity.RoleBroadcaster)
	ctx := context.Background()

	created := seedTrack(t, repo, user.ID, "Nocturne")

	if created.ID == "" {
		t.Fatal("Create did not assign an ID")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("Create did not return the timestamps written by the database")
	}

	got, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.Title != "Nocturne" || got.SizeBytes != 4096 || got.ContentType != "audio/mpeg" {
		t.Errorf("FindByID() = %+v, unexpected", got)
	}
	// The join is the part a fake cannot check: the username comes from the
	// users table, not from the tracks row.
	if got.UploaderUsername != user.Username {
		t.Errorf("UploaderUsername = %q, want %q — the join is not wired", got.UploaderUsername, user.Username)
	}
}

func TestTrackRepository_FindMissing_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewTrackRepository(db)

	_, err := repo.FindByID(context.Background(), uuid.NewString())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("FindByID() on an unknown id = %v, want ErrNotFound", err)
	}
}

func TestTrackRepository_DuplicateStorageKeyIsAConflict_Integration(t *testing.T) {
	// storage_key is UNIQUE: two rows pointing at the same object would let
	// deleting one strand the other on bytes that are already gone.
	db := openMigratedDB(t)
	repo := NewTrackRepository(db)
	user := seedUser(t, db, entity.RoleBroadcaster)

	first := seedTrack(t, repo, user.ID, "Nocturne")

	duplicate := &entity.AudioTrack{
		Title:       "Autre titre",
		StorageKey:  first.StorageKey,
		ContentType: "audio/mpeg",
		SizeBytes:   1,
		UploaderID:  user.ID,
	}
	if err := repo.Create(context.Background(), duplicate); !errors.Is(err, repository.ErrConflict) {
		t.Errorf("Create() with a duplicate storage_key = %v, want ErrConflict", err)
	}
}

func TestTrackRepository_UnknownUploaderIsNotFound_Integration(t *testing.T) {
	// The foreign key on uploader_id is what stops a track from outliving
	// the account it belongs to.
	db := openMigratedDB(t)
	repo := NewTrackRepository(db)

	orphan := &entity.AudioTrack{
		Title:       "Sans proprietaire",
		StorageKey:  uuid.NewString() + ".mp3",
		ContentType: "audio/mpeg",
		SizeBytes:   1,
		UploaderID:  uuid.NewString(),
	}
	if err := repo.Create(context.Background(), orphan); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Create() with an unknown uploader = %v, want ErrNotFound", err)
	}
}

func TestTrackRepository_ListAndPagination_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewTrackRepository(db)
	user := seedUser(t, db, entity.RoleBroadcaster)
	ctx := context.Background()

	for _, title := range []string{"Un", "Deux", "Trois"} {
		seedTrack(t, repo, user.ID, title)
	}

	all, err := repo.List(ctx, 100, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	mine := 0
	for _, tr := range all {
		if tr.UploaderID == user.ID {
			mine++
		}
	}
	if mine != 3 {
		t.Errorf("List() returned %d of this uploader's tracks, want 3", mine)
	}

	page, err := repo.List(ctx, 2, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page) > 2 {
		t.Errorf("List(limit=2) returned %d rows, want at most 2", len(page))
	}

	// LIMIT/OFFSET past the end must be an empty page, not an error.
	beyond, err := repo.List(ctx, 10, 100000)
	if err != nil {
		t.Fatalf("List() beyond the end error = %v", err)
	}
	if len(beyond) != 0 {
		t.Errorf("List() beyond the end returned %d rows, want 0", len(beyond))
	}
}

func TestTrackRepository_ListByUploaderIsScoped_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewTrackRepository(db)
	mine := seedUser(t, db, entity.RoleBroadcaster)
	theirs := seedUser(t, db, entity.RoleBroadcaster)

	seedTrack(t, repo, mine.ID, "A moi")
	seedTrack(t, repo, theirs.ID, "A eux")

	got, err := repo.ListByUploader(context.Background(), mine.ID)
	if err != nil {
		t.Fatalf("ListByUploader() error = %v", err)
	}
	if len(got) != 1 || got[0].UploaderID != mine.ID {
		t.Errorf("ListByUploader() = %+v, want only this uploader's track", got)
	}
}

func TestTrackRepository_Delete_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewTrackRepository(db)
	user := seedUser(t, db, entity.RoleBroadcaster)
	ctx := context.Background()

	track := seedTrack(t, repo, user.ID, "A supprimer")

	if err := repo.Delete(ctx, track.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := repo.FindByID(ctx, track.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("FindByID() after Delete = %v, want ErrNotFound", err)
	}
	// Unlike users (whose deletion is an RGPD erasure and therefore
	// idempotent), deleting a track that is not there is a caller bug: the
	// row is the source of truth for an object that still exists on disk.
	if err := repo.Delete(ctx, track.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("second Delete() = %v, want ErrNotFound", err)
	}
}

func TestTrackRepository_DeletingTheUploaderCascades_Integration(t *testing.T) {
	// This is what makes the RGPD account deletion (ticket Y2) remove a
	// user's tracks without Y2 having to know this table exists. The stored
	// objects are NOT removed by the cascade — see ADR 0009.
	db := openMigratedDB(t)
	repo := NewTrackRepository(db)
	user := seedUser(t, db, entity.RoleBroadcaster)
	ctx := context.Background()

	track := seedTrack(t, repo, user.ID, "Cascade")

	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("delete uploader: %v", err)
	}

	if _, err := repo.FindByID(ctx, track.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("the track outlived its uploader: FindByID() = %v, want ErrNotFound", err)
	}
}
