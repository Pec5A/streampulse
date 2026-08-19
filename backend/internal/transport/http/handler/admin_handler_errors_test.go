package handler_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

// erroringAdminRepo fails every call, so the handler's 500 paths are exercised.
type erroringAdminRepo struct{}

func (erroringAdminRepo) ListUsers(context.Context, int, int) ([]entity.User, error) {
	return nil, errors.New("db down")
}
func (erroringAdminRepo) CountUsersByRole(context.Context) (map[entity.Role]int, error) {
	return nil, errors.New("db down")
}
func (erroringAdminRepo) FindByID(context.Context, string) (*entity.User, error) {
	return nil, errors.New("db down")
}
func (erroringAdminRepo) Update(context.Context, *entity.User) error {
	return errors.New("db down")
}

var _ repository.AdminUserRepository = erroringAdminRepo{}

func TestAdminAPI_RepoErrorsAre500(t *testing.T) {
	srv, jwt := newAdminServer(erroringAdminRepo{})
	admin := adminToken(t, jwt, "a", "admin")

	if rec := adminReq(t, srv, http.MethodGet, "/api/v1/admin/stats", admin, nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("stats error -> %d, want 500", rec.Code)
	}
	if rec := adminReq(t, srv, http.MethodGet, "/api/v1/admin/users", admin, nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("list error -> %d, want 500", rec.Code)
	}
}
