package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func newServerWithRepo(repo repository.PlaylistRepository) (http.Handler, *auth.JWTManager) {
	uc := usecase.NewPlaylistUseCase(repo)
	jwt := auth.NewJWTManager("test-secret-at-least-32-bytes-long!!", time.Hour)
	return router.New(router.Handlers{Playlist: handler.NewPlaylistHandler(uc)}, jwt), jwt
}

// hookRepo wraps memRepo to force errors on specific operations, exercising the
// handler's error mapping (500 default, 409 conflict).
type hookRepo struct {
	*memRepo
	createErr   error
	addTrackErr error
}

func (r *hookRepo) Create(ctx context.Context, p *entity.Playlist) error {
	if r.createErr != nil {
		return r.createErr
	}
	return r.memRepo.Create(ctx, p)
}

func (r *hookRepo) AddTrack(ctx context.Context, t *entity.Track) error {
	if r.addTrackErr != nil {
		return r.addTrackErr
	}
	return r.memRepo.AddTrack(ctx, t)
}

var _ repository.PlaylistRepository = (*hookRepo)(nil)

func TestPlaylistAPI_BadJSONBodies(t *testing.T) {
	srv, jwt := newServer()
	tok := tokenFor(t, jwt, "alice")
	rec := do(t, srv, http.MethodPost, "/api/v1/playlists", tok, dto.CreatePlaylistRequest{Name: "P"})
	p := decodeBody[dto.PlaylistResponse](t, rec)

	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/playlists"},
		{http.MethodPatch, "/api/v1/playlists/" + p.ID},
		{http.MethodPost, "/api/v1/playlists/" + p.ID + "/tracks"},
		{http.MethodPut, "/api/v1/playlists/" + p.ID + "/tracks/order"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{ not json"))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s %s with bad json -> %d, want 400", tc.method, tc.path, w.Code)
		}
	}
}

func TestPlaylistAPI_AddTrackValidation(t *testing.T) {
	srv, jwt := newServer()
	tok := tokenFor(t, jwt, "alice")
	rec := do(t, srv, http.MethodPost, "/api/v1/playlists", tok, dto.CreatePlaylistRequest{Name: "P"})
	p := decodeBody[dto.PlaylistResponse](t, rec)

	if rec := do(t, srv, http.MethodPost, "/api/v1/playlists/"+p.ID+"/tracks", tok, dto.AddTrackRequest{Title: "  "}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty title -> %d, want 400", rec.Code)
	}
}

func TestPlaylistAPI_RepoErrorsMapToStatus(t *testing.T) {
	// A generic repo failure surfaces as 500.
	broken := &hookRepo{memRepo: newMemRepo(), createErr: errors.New("db down")}
	srv, jwt := newServerWithRepo(broken)
	tok := tokenFor(t, jwt, "alice")
	if rec := do(t, srv, http.MethodPost, "/api/v1/playlists", tok, dto.CreatePlaylistRequest{Name: "P"}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("create with broken repo -> %d, want 500", rec.Code)
	}

	// A position conflict on AddTrack surfaces as 409.
	conflict := &hookRepo{memRepo: newMemRepo(), addTrackErr: repository.ErrConflict}
	srv2, jwt2 := newServerWithRepo(conflict)
	tok2 := tokenFor(t, jwt2, "alice")
	rec := do(t, srv2, http.MethodPost, "/api/v1/playlists", tok2, dto.CreatePlaylistRequest{Name: "P"})
	p := decodeBody[dto.PlaylistResponse](t, rec)
	if rec := do(t, srv2, http.MethodPost, "/api/v1/playlists/"+p.ID+"/tracks", tok2, dto.AddTrackRequest{Title: "x"}); rec.Code != http.StatusConflict {
		t.Fatalf("add track conflict -> %d, want 409", rec.Code)
	}
}

// TestPlaylistHandler_MissingAuthContext calls handlers directly (bypassing the
// auth middleware) so the authenticated-user lookup fails and returns 401.
func TestPlaylistHandler_MissingAuthContext(t *testing.T) {
	h := handler.NewPlaylistHandler(usecase.NewPlaylistUseCase(newMemRepo()))
	for name, fn := range map[string]http.HandlerFunc{"Create": h.Create, "List": h.List} {
		w := httptest.NewRecorder()
		fn(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s without auth context -> %d, want 401", name, w.Code)
		}
	}
}
