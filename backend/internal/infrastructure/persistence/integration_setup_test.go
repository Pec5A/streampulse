//go:build integration

package persistence

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/streampulse/backend/internal/domain/entity"
)

// openMigratedDB opens the database named by DATABASE_URL and brings its
// schema up to date, skipping the test when the variable is unset.
//
// Applying the migrations here, rather than assuming somebody applied them,
// is the entire point. The previous version of these tests only passed when
// pointed — by luck — at a database that an API run had already migrated.
// Against the fresh Postgres that CI hands you, the first one failed with
// `relation "users" does not exist`, and nobody saw it: the `integration`
// build tag meant CI never ran them at all, so the rot was invisible.
//
// A test that establishes its own state is also a test anyone can run on a
// throwaway container without a setup ritual to remember.
func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ping first: without it a wrong DSN surfaces as a confusing migration
	// error instead of "cannot reach the database".
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	return db
}

// seedUser inserts a user with a unique email/username and removes it (and
// everything cascading from it) when the test ends.
//
// Every test gets its own user rather than sharing a fixture: these tests run
// against the same database, and a shared row makes failures depend on
// execution order — the kind of flake that only shows up in CI.
func seedUser(t *testing.T, db *sql.DB, role entity.Role) *entity.User {
	t.Helper()

	suffix := uuid.NewString()[:8]
	u := &entity.User{
		Email:    "user-" + suffix + "@test.local",
		Username: "user-" + suffix,
		Password: "not-a-real-hash",
		Role:     role,
	}

	repo := NewUserRepository(db)
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, u.ID)
	})

	return u
}
