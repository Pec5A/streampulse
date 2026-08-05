package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// memRepo is an in-memory repository.PlaylistRepository for black-box handler
// tests (drives real usecase + router + auth middleware, no database).
type memRepo struct {
	mu        sync.Mutex
	playlists map[string]*entity.Playlist
	tracks    map[string][]entity.Track
	seq       int
}

func newMemRepo() *memRepo {
	return &memRepo{playlists: map[string]*entity.Playlist{}, tracks: map[string][]entity.Track{}}
}

func (r *memRepo) id(p string) string { r.seq++; return fmt.Sprintf("%s-%d", p, r.seq) }

func (r *memRepo) Create(_ context.Context, p *entity.Playlist) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = r.id("pl")
	}
	now := time.Now()
	p.CreatedAt, p.UpdatedAt = now, now
	cp := *p
	r.playlists[p.ID] = &cp
	return nil
}

func (r *memRepo) FindByID(_ context.Context, id string) (*entity.Playlist, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.playlists[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, repository.ErrNotFound
}

func (r *memRepo) ListByOwner(_ context.Context, ownerID string) ([]entity.Playlist, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []entity.Playlist
	for _, p := range r.playlists {
		if p.OwnerID == ownerID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *memRepo) Update(_ context.Context, p *entity.Playlist) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.playlists[p.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *p
	r.playlists[p.ID] = &cp
	return nil
}

func (r *memRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.playlists[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.playlists, id)
	delete(r.tracks, id)
	return nil
}

func (r *memRepo) ListTracks(_ context.Context, playlistID string) ([]entity.Track, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]entity.Track(nil), r.tracks[playlistID]...), nil
}

func (r *memRepo) AddTrack(_ context.Context, t *entity.Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = r.id("tr")
	}
	t.Position = len(r.tracks[t.PlaylistID])
	r.tracks[t.PlaylistID] = append(r.tracks[t.PlaylistID], *t)
	return nil
}

func (r *memRepo) RemoveTrack(_ context.Context, playlistID, trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ts := r.tracks[playlistID]
	out := make([]entity.Track, 0, len(ts))
	found := false
	for _, t := range ts {
		if t.ID == trackID {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return repository.ErrNotFound
	}
	for i := range out {
		out[i].Position = i
	}
	r.tracks[playlistID] = out
	return nil
}

func (r *memRepo) ReorderTracks(_ context.Context, playlistID string, ordered []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	byID := map[string]entity.Track{}
	for _, t := range r.tracks[playlistID] {
		byID[t.ID] = t
	}
	out := make([]entity.Track, 0, len(ordered))
	for i, id := range ordered {
		t, ok := byID[id]
		if !ok {
			return repository.ErrNotFound
		}
		t.Position = i
		out = append(out, t)
	}
	r.tracks[playlistID] = out
	return nil
}

var _ repository.PlaylistRepository = (*memRepo)(nil)

// ---- test harness ----

func newServer() (http.Handler, *auth.JWTManager) {
	uc := usecase.NewPlaylistUseCase(newMemRepo())
	ph := handler.NewPlaylistHandler(uc)
	jwt := auth.NewJWTManager("test-secret-at-least-32-bytes-long!!", time.Hour)
	return router.New(router.Handlers{Playlist: ph}, jwt), jwt
}

func tokenFor(t *testing.T, jwt *auth.JWTManager, userID string) string {
	t.Helper()
	tok, err := jwt.Generate(userID, "user")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func do(t *testing.T, srv http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	return v
}

// ---- tests ----

func TestPlaylistAPI_Unauthenticated(t *testing.T) {
	srv, _ := newServer()
	if rec := do(t, srv, http.MethodGet, "/api/v1/playlists", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestPlaylistAPI_CreateAndList(t *testing.T) {
	srv, jwt := newServer()
	tok := tokenFor(t, jwt, "alice")

	rec := do(t, srv, http.MethodPost, "/api/v1/playlists", tok, dto.CreatePlaylistRequest{Name: "Focus", Description: "deep work"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code = %d, body = %s", rec.Code, rec.Body)
	}
	created := decodeBody[dto.PlaylistResponse](t, rec)
	if created.ID == "" || created.Name != "Focus" || created.OwnerID != "alice" {
		t.Fatalf("bad created playlist: %+v", created)
	}

	rec = do(t, srv, http.MethodGet, "/api/v1/playlists", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list code = %d", rec.Code)
	}
	if list := decodeBody[[]dto.PlaylistResponse](t, rec); len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
}

func TestPlaylistAPI_CreateValidation(t *testing.T) {
	srv, jwt := newServer()
	tok := tokenFor(t, jwt, "alice")
	if rec := do(t, srv, http.MethodPost, "/api/v1/playlists", tok, dto.CreatePlaylistRequest{Name: "   "}); rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestPlaylistAPI_GetForbiddenAndNotFound(t *testing.T) {
	srv, jwt := newServer()
	alice := tokenFor(t, jwt, "alice")
	bob := tokenFor(t, jwt, "bob")

	rec := do(t, srv, http.MethodPost, "/api/v1/playlists", alice, dto.CreatePlaylistRequest{Name: "Private"})
	p := decodeBody[dto.PlaylistResponse](t, rec)

	if rec := do(t, srv, http.MethodGet, "/api/v1/playlists/"+p.ID, bob, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("bob get code = %d, want 403", rec.Code)
	}
	if rec := do(t, srv, http.MethodGet, "/api/v1/playlists/missing", alice, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing get code = %d, want 404", rec.Code)
	}
}

func TestPlaylistAPI_UpdateAndDelete(t *testing.T) {
	srv, jwt := newServer()
	alice := tokenFor(t, jwt, "alice")
	bob := tokenFor(t, jwt, "bob")

	rec := do(t, srv, http.MethodPost, "/api/v1/playlists", alice, dto.CreatePlaylistRequest{Name: "Mix"})
	p := decodeBody[dto.PlaylistResponse](t, rec)

	name := "Renamed"
	rec = do(t, srv, http.MethodPatch, "/api/v1/playlists/"+p.ID, alice, dto.UpdatePlaylistRequest{Name: &name})
	if rec.Code != http.StatusOK || decodeBody[dto.PlaylistResponse](t, rec).Name != "Renamed" {
		t.Fatalf("update code = %d, body = %s", rec.Code, rec.Body)
	}

	if rec := do(t, srv, http.MethodPatch, "/api/v1/playlists/"+p.ID, bob, dto.UpdatePlaylistRequest{Name: &name}); rec.Code != http.StatusForbidden {
		t.Fatalf("bob update code = %d, want 403", rec.Code)
	}

	if rec := do(t, srv, http.MethodDelete, "/api/v1/playlists/"+p.ID, alice, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete code = %d, want 204", rec.Code)
	}
	if rec := do(t, srv, http.MethodGet, "/api/v1/playlists/"+p.ID, alice, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete code = %d, want 404", rec.Code)
	}
}

func TestPlaylistAPI_TracksAddRemoveReorder(t *testing.T) {
	srv, jwt := newServer()
	alice := tokenFor(t, jwt, "alice")

	rec := do(t, srv, http.MethodPost, "/api/v1/playlists", alice, dto.CreatePlaylistRequest{Name: "Queue"})
	p := decodeBody[dto.PlaylistResponse](t, rec)

	add := func(title string) dto.TrackResponse {
		rec := do(t, srv, http.MethodPost, "/api/v1/playlists/"+p.ID+"/tracks", alice, dto.AddTrackRequest{Title: title})
		if rec.Code != http.StatusCreated {
			t.Fatalf("add %q code = %d, body = %s", title, rec.Code, rec.Body)
		}
		return decodeBody[dto.TrackResponse](t, rec)
	}
	a, b, c := add("A"), add("B"), add("C")
	if a.Position != 0 || b.Position != 1 || c.Position != 2 {
		t.Fatalf("positions = %d,%d,%d, want 0,1,2", a.Position, b.Position, c.Position)
	}

	rec = do(t, srv, http.MethodPut, "/api/v1/playlists/"+p.ID+"/tracks/order", alice,
		dto.ReorderRequest{TrackIDs: []string{c.ID, a.ID, b.ID}})
	if rec.Code != http.StatusOK {
		t.Fatalf("reorder code = %d, body = %s", rec.Code, rec.Body)
	}
	full := decodeBody[dto.PlaylistWithTracksResponse](t, rec)
	if got := []string{full.Tracks[0].Title, full.Tracks[1].Title, full.Tracks[2].Title}; got[0] != "C" || got[1] != "A" || got[2] != "B" {
		t.Fatalf("order = %v, want [C A B]", got)
	}

	// A reorder list that isn't the exact track set is rejected.
	if rec := do(t, srv, http.MethodPut, "/api/v1/playlists/"+p.ID+"/tracks/order", alice,
		dto.ReorderRequest{TrackIDs: []string{a.ID}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid reorder code = %d, want 400", rec.Code)
	}

	if rec := do(t, srv, http.MethodDelete, "/api/v1/playlists/"+p.ID+"/tracks/"+b.ID, alice, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("remove code = %d, want 204", rec.Code)
	}
	rec = do(t, srv, http.MethodGet, "/api/v1/playlists/"+p.ID, alice, nil)
	full = decodeBody[dto.PlaylistWithTracksResponse](t, rec)
	if len(full.Tracks) != 2 || full.Tracks[0].Position != 0 || full.Tracks[1].Position != 1 {
		t.Fatalf("after remove: %d tracks, positions %d,%d", len(full.Tracks), full.Tracks[0].Position, full.Tracks[1].Position)
	}
}
