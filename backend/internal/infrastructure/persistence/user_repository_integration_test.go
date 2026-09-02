//go:build integration

// Integration test for the pgx UserRepository. Behind the `integration` tag
// because it needs a real Postgres; CI provides one and runs it. Locally:
//
//	DATABASE_URL=postgres://user:pass@localhost:5432/streampulse?sslmode=disable \
//	  go test -tags integration ./internal/infrastructure/persistence/
package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

// TestUserRepository_Delete_IdempotentOnMissingRow guards against exactly
// the divergence a review caught: fakeUserRepo.Delete was already
// idempotent on a missing id, but the real pgx repository used to call
// checkRowsAffected and return repository.ErrNotFound on the second call —
// a real RGPD-erasure retry (the caller's JWT stays valid after the row is
// gone) would 500 instead of behaving like the first, successful call.
// Unit tests against the fake can't catch this: only the real repository's
// SQL behavior is under test here.
func TestUserRepository_Delete_IdempotentOnMissingRow(t *testing.T) {
	repo := NewUserRepository(openMigratedDB(t))
	ctx := context.Background()

	if err := repo.Delete(ctx, "00000000-0000-0000-0000-000000000000"); err != nil {
		t.Fatalf("Delete() on a never-existing id error = %v, want nil (idempotent)", err)
	}
}

func TestUserRepository_CreateThenFind_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u := seedUser(t, db, entity.RoleUser)
	if u.ID == "" {
		t.Fatal("Create() left ID empty; the generated uuid must be read back")
	}
	if u.CreatedAt.IsZero() {
		t.Error("Create() left CreatedAt zero; the DB default must be read back")
	}

	byID, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if byID.Email != u.Email || byID.Role != entity.RoleUser {
		t.Errorf("FindByID() = %+v, want email %q and role user", byID, u.Email)
	}

	byEmail, err := repo.FindByEmail(ctx, u.Email)
	if err != nil {
		t.Fatalf("FindByEmail() error = %v", err)
	}
	if byEmail.ID != u.ID {
		t.Errorf("FindByEmail() returned id %q, want %q", byEmail.ID, u.ID)
	}
	// The hash must come back: this is the row Login compares against.
	if byEmail.Password != "not-a-real-hash" {
		t.Errorf("FindByEmail() password = %q, want the stored hash", byEmail.Password)
	}
}

func TestUserRepository_FindMissing_Integration(t *testing.T) {
	repo := NewUserRepository(openMigratedDB(t))
	ctx := context.Background()

	// A well-formed uuid that exists nowhere: the point is to exercise the
	// not-found path, not to make Postgres reject a malformed cast.
	if _, err := repo.FindByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("FindByID(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByEmail(ctx, "nobody-"+uuid.NewString()+"@test.local"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("FindByEmail(missing) error = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_DuplicateEmailIsAConflict_Integration(t *testing.T) {
	// The unique constraint lives in the schema, so only a real database can
	// tell us the driver error is translated into ErrConflict rather than
	// surfacing as a 500.
	db := openMigratedDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	first := seedUser(t, db, entity.RoleUser)

	dup := &entity.User{
		Email:    first.Email,
		Username: "other-" + uuid.NewString()[:8],
		Password: "not-a-real-hash",
		Role:     entity.RoleUser,
	}
	if err := repo.Create(ctx, dup); !errors.Is(err, repository.ErrConflict) {
		t.Errorf("Create(duplicate email) error = %v, want ErrConflict", err)
	}
}

func TestUserRepository_Update_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u := seedUser(t, db, entity.RoleUser)
	u.Username = "renamed-" + uuid.NewString()[:8]
	u.Role = entity.RoleBroadcaster

	if err := repo.Update(ctx, u); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.Username != u.Username || got.Role != entity.RoleBroadcaster {
		t.Errorf("after Update(): username %q role %q, want %q and broadcaster", got.Username, got.Role, u.Username)
	}
}

func TestUserRepository_UpdateMissingIsNotFound_Integration(t *testing.T) {
	// Deliberately asymmetric with Delete: a no-op Update signals a caller
	// bug, a no-op Delete leaves the desired end state. See Delete's comment.
	repo := NewUserRepository(openMigratedDB(t))

	err := repo.Update(context.Background(), &entity.User{
		ID:       "00000000-0000-0000-0000-000000000000",
		Email:    "ghost-" + uuid.NewString()[:8] + "@test.local",
		Username: "ghost-" + uuid.NewString()[:8],
		Role:     entity.RoleUser,
	})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Update(missing) error = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_DeleteThenFind_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u := seedUser(t, db, entity.RoleUser)
	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := repo.FindByID(ctx, u.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("FindByID() after Delete() error = %v, want ErrNotFound", err)
	}
	// Second call: the RGPD retry path.
	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Errorf("second Delete() error = %v, want nil (idempotent)", err)
	}
}

func TestUserRepository_ListUsersAndCountByRole_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	admin := seedUser(t, db, entity.RoleAdmin)
	broadcaster := seedUser(t, db, entity.RoleBroadcaster)

	// Newest first, so the two users just created are on the first page.
	users, err := repo.ListUsers(ctx, 0, 50)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	seen := map[string]entity.Role{}
	for _, u := range users {
		seen[u.ID] = u.Role
	}
	if seen[admin.ID] != entity.RoleAdmin {
		t.Errorf("ListUsers() did not return the seeded admin with its role, got %q", seen[admin.ID])
	}
	if seen[broadcaster.ID] != entity.RoleBroadcaster {
		t.Errorf("ListUsers() did not return the seeded broadcaster with its role, got %q", seen[broadcaster.ID])
	}

	counts, err := repo.CountUsersByRole(ctx)
	if err != nil {
		t.Fatalf("CountUsersByRole() error = %v", err)
	}
	// Absolute counts depend on what other tests left behind, so assert the
	// only thing that is actually invariant: our rows are represented.
	if counts[entity.RoleAdmin] < 1 || counts[entity.RoleBroadcaster] < 1 {
		t.Errorf("CountUsersByRole() = %v, want at least one admin and one broadcaster", counts)
	}
}

func TestUserRepository_ListUsersClampsAbsurdPaging_Integration(t *testing.T) {
	db := openMigratedDB(t)
	repo := NewUserRepository(db)
	seedUser(t, db, entity.RoleUser)

	// A negative offset or a huge limit must not reach Postgres as-is:
	// OFFSET -1 is a hard SQL error, and an unbounded LIMIT is a way to make
	// an admin endpoint dump the whole table.
	users, err := repo.ListUsers(context.Background(), -10, 100000)
	if err != nil {
		t.Fatalf("ListUsers(-10, 100000) error = %v, want the values to be clamped", err)
	}
	if len(users) > 200 {
		t.Errorf("ListUsers() returned %d rows, want the limit clamped", len(users))
	}
}
