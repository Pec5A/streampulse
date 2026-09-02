//go:build integration

// Integration test for the pgx UserRepository. Excluded from the default
// build/CI (no database there); run it against a real Postgres with:
//
//	DATABASE_URL=postgres://user:pass@localhost:5432/streampulse?sslmode=disable \
//	  go test -tags integration ./internal/infrastructure/persistence/
//
// Requires migration 0001 to have been applied.
package persistence

import (
	"context"
	"database/sql"
	"os"
	"testing"
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
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewUserRepository(db)
	ctx := context.Background()

	if err := repo.Delete(ctx, "00000000-0000-0000-0000-000000000000"); err != nil {
		t.Fatalf("Delete() on a never-existing id error = %v, want nil (idempotent)", err)
	}
}
