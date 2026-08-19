package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/application/dto"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/transport/http/handler"
	"github.com/streampulse/backend/internal/transport/http/router"
)

type adminMemRepo struct {
	mu    sync.Mutex
	users map[string]*entity.User
}

func newAdminMemRepo(users ...*entity.User) *adminMemRepo {
	m := make(map[string]*entity.User, len(users))
	for _, u := range users {
		cp := *u
		m[u.ID] = &cp
	}
	return &adminMemRepo{users: m}
}

func (r *adminMemRepo) ListUsers(_ context.Context, _, _ int) ([]entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, *u)
	}
	return out, nil
}

func (r *adminMemRepo) CountUsersByRole(_ context.Context) (map[entity.Role]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[entity.Role]int{}
	for _, u := range r.users {
		out[u.Role]++
	}
	return out, nil
}

func (r *adminMemRepo) FindByID(_ context.Context, id string) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, repository.ErrNotFound
}

func (r *adminMemRepo) Update(_ context.Context, u *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *u
	r.users[u.ID] = &cp
	return nil
}

var _ repository.AdminUserRepository = (*adminMemRepo)(nil)

func newAdminServer(repo repository.AdminUserRepository) (http.Handler, *auth.JWTManager) {
	uc := usecase.NewAdminUseCase(repo)
	jwt := auth.NewJWTManager("test-secret-at-least-32-bytes-long!!", time.Hour)
	return router.New(router.Handlers{Admin: handler.NewAdminHandler(uc)}, jwt), jwt
}

func adminReq(t *testing.T, srv http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func adminToken(t *testing.T, jwt *auth.JWTManager, id, role string) string {
	t.Helper()
	s, err := jwt.Generate(id, role)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return s
}

func TestAdminAPI_AccessControl(t *testing.T) {
	srv, jwt := newAdminServer(newAdminMemRepo())

	if rec := adminReq(t, srv, http.MethodGet, "/api/v1/admin/stats", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token -> %d, want 401", rec.Code)
	}
	if rec := adminReq(t, srv, http.MethodGet, "/api/v1/admin/stats", adminToken(t, jwt, "u", "user"), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("user token -> %d, want 403", rec.Code)
	}
}

func TestAdminAPI_StatsAndList(t *testing.T) {
	srv, jwt := newAdminServer(newAdminMemRepo(
		&entity.User{ID: "1", Email: "a@x", Username: "a", Role: entity.RoleUser},
		&entity.User{ID: "2", Email: "b@x", Username: "b", Role: entity.RoleAdmin},
	))
	admin := adminToken(t, jwt, "2", "admin")

	rec := adminReq(t, srv, http.MethodGet, "/api/v1/admin/stats", admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats -> %d", rec.Code)
	}
	var s dto.StatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if s.TotalUsers != 2 || s.TotalAdmins != 1 || s.TotalRegular != 1 {
		t.Fatalf("stats = %+v", s)
	}

	rec = adminReq(t, srv, http.MethodGet, "/api/v1/admin/users", admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list -> %d", rec.Code)
	}
	var users []dto.UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("users len = %d, want 2", len(users))
	}
}

func TestAdminAPI_UpdateRole(t *testing.T) {
	srv, jwt := newAdminServer(newAdminMemRepo(
		&entity.User{ID: "1", Email: "a@x", Username: "a", Role: entity.RoleUser},
		&entity.User{ID: "admin", Role: entity.RoleAdmin},
	))
	admin := adminToken(t, jwt, "admin", "admin")

	rec := adminReq(t, srv, http.MethodPatch, "/api/v1/admin/users/1/role", admin, dto.UpdateRoleRequest{Role: "broadcaster"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update -> %d body=%s", rec.Code, rec.Body)
	}
	var u dto.UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	if u.Role != "broadcaster" {
		t.Fatalf("role = %q, want broadcaster", u.Role)
	}

	if rec := adminReq(t, srv, http.MethodPatch, "/api/v1/admin/users/1/role", admin, dto.UpdateRoleRequest{Role: "root"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid role -> %d, want 400", rec.Code)
	}
	if rec := adminReq(t, srv, http.MethodPatch, "/api/v1/admin/users/ghost/role", admin, dto.UpdateRoleRequest{Role: "admin"}); rec.Code != http.StatusNotFound {
		t.Fatalf("missing user -> %d, want 404", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/1/role", bytes.NewReader([]byte("{bad")))
	req.Header.Set("Authorization", "Bearer "+admin)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad json -> %d, want 400", w.Code)
	}
}
