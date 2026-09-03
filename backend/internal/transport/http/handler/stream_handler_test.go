package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/streampulse/backend/internal/application/dto"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/infrastructure/observability"
	"github.com/streampulse/backend/internal/infrastructure/streaming"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

const testSecret = "test-secret-at-least-32-bytes-long"

// stubStreamRepo is an in-memory repository.StreamRepository. Handler tests
// go through the real use case rather than a mocked one, so a wiring or
// status-code mistake fails here instead of in production.
type stubStreamRepo struct {
	mu      sync.Mutex
	byID    map[string]*entity.Stream
	counter int

	// failList makes the list queries return an error, to check the handler
	// reports an outage instead of an empty catalogue.
	failList error
}

func newStubStreamRepo() *stubStreamRepo {
	return &stubStreamRepo{byID: map[string]*entity.Stream{}}
}

func (r *stubStreamRepo) Create(_ context.Context, s *entity.Stream) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counter++
	if s.ID == "" {
		s.ID = "stream-" + string(rune('a'-1+r.counter))
	}
	clone := *s
	r.byID[s.ID] = &clone
	return nil
}

func (r *stubStreamRepo) FindByID(_ context.Context, id string) (*entity.Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byID[id]; ok {
		clone := *s
		return &clone, nil
	}
	return nil, repository.ErrNotFound
}

func (r *stubStreamRepo) List(_ context.Context) ([]entity.Stream, error) {
	if r.failList != nil {
		return nil, r.failList
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Stream, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, *s)
	}
	return out, nil
}

func (r *stubStreamRepo) ListByStatus(_ context.Context, status entity.StreamStatus) ([]entity.Stream, error) {
	if r.failList != nil {
		return nil, r.failList
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Stream, 0)
	for _, s := range r.byID {
		if s.Status == status {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (r *stubStreamRepo) UpdateStatus(_ context.Context, id string, status entity.StreamStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	s.Status = status
	return nil
}

func (r *stubStreamRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.byID, id)
	return nil
}

var _ repository.StreamRepository = (*stubStreamRepo)(nil)

// streamTestRig wires a real use case, a real JWT manager and a mux that
// mirrors the production routes, so path values and middleware behave as they
// do at runtime.
type streamTestRig struct {
	mux      *http.ServeMux
	repo     *stubStreamRepo
	users    *fakeUserRepo
	registry *streaming.Registry
	chats    *streaming.ChatRegistry
	jwt      *auth.JWTManager
}

func newStreamTestRig(t *testing.T) *streamTestRig {
	t.Helper()

	repo := newStubStreamRepo()
	users := newFakeUserRepo()
	registry := streaming.NewRegistry()
	t.Cleanup(registry.CloseAll)
	chats := streaming.NewChatRegistry()
	t.Cleanup(chats.CloseAll)

	uc := usecase.NewStreamUseCase(repo, users, registry, chats)
	h := NewStreamHandler(uc)
	jwtManager := auth.NewJWTManager(testSecret, time.Hour)
	authed := middleware.RequireAuth(jwtManager)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/streams", h.List)
	mux.HandleFunc("GET /api/v1/streams/live", h.ListLive)
	mux.HandleFunc("GET /api/v1/streams/{id}", h.Get)
	mux.HandleFunc("GET /api/v1/streams/{id}/listen", h.Listen)
	mux.Handle("POST /api/v1/streams", authed(http.HandlerFunc(h.Create)))
	mux.Handle("DELETE /api/v1/streams/{id}", authed(http.HandlerFunc(h.Delete)))
	mux.Handle("POST /api/v1/streams/{id}/publish", authed(http.HandlerFunc(h.Publish)))
	mux.Handle("GET /api/v1/streams/{id}/chat",
		middleware.RequireAuthWS(jwtManager)(http.HandlerFunc(h.Chat)))

	return &streamTestRig{mux: mux, repo: repo, users: users, registry: registry, chats: chats, jwt: jwtManager}
}

// token mints a real JWT, so tests exercise the actual auth path.
func (rig *streamTestRig) token(t *testing.T, userID, role string) string {
	t.Helper()
	tok, err := rig.jwt.Generate(userID, role)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func (rig *streamTestRig) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	rig.mux.ServeHTTP(rec, req)
	return rec
}

// seed inserts a stream directly, bypassing HTTP.
func (rig *streamTestRig) seed(t *testing.T, owner, title string) *entity.Stream {
	t.Helper()
	s := &entity.Stream{Title: title, BroadcasterID: owner, Status: entity.StreamStatusOffline}
	if err := rig.repo.Create(context.Background(), s); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	return s
}

func decodeStream(t *testing.T, rec *httptest.ResponseRecorder) dto.StreamResponse {
	t.Helper()
	var got dto.StreamResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return got
}

func TestStreamHandler_CreateRequiresAuth(t *testing.T) {
	rig := newStreamTestRig(t)

	rec := rig.do(t, http.MethodPost, "/api/v1/streams", "", dto.CreateStreamRequest{Title: "Jazz"})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestStreamHandler_Create(t *testing.T) {
	rig := newStreamTestRig(t)
	token := rig.token(t, "user-1", string(entity.RoleUser))

	rec := rig.do(t, http.MethodPost, "/api/v1/streams", token,
		dto.CreateStreamRequest{Title: "Jazz de nuit", Description: "session live"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body)
	}
	got := decodeStream(t, rec)
	if got.Title != "Jazz de nuit" {
		t.Errorf("Title = %q, want %q", got.Title, "Jazz de nuit")
	}
	if got.BroadcasterID != "user-1" {
		t.Errorf("BroadcasterID = %q, want user-1", got.BroadcasterID)
	}
	if got.Status != string(entity.StreamStatusOffline) {
		t.Errorf("Status = %q, want offline", got.Status)
	}
	if got.ListenerCount != 0 {
		t.Errorf("ListenerCount = %d, want 0", got.ListenerCount)
	}
}

func TestStreamHandler_CreateRejectsBadInput(t *testing.T) {
	rig := newStreamTestRig(t)
	token := rig.token(t, "user-1", string(entity.RoleUser))

	t.Run("malformed json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/streams", bytes.NewBufferString("{nope"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		rig.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("empty title", func(t *testing.T) {
		rec := rig.do(t, http.MethodPost, "/api/v1/streams", token, dto.CreateStreamRequest{Title: "  "})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestStreamHandler_GetIncludesLiveListenerCount(t *testing.T) {
	rig := newStreamTestRig(t)
	s := rig.seed(t, "user-1", "Jazz")

	hub := rig.registry.Open(context.Background(), s.ID)
	for range 3 {
		if _, _, _, err := hub.Subscribe(); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}

	rec := rig.do(t, http.MethodGet, "/api/v1/streams/"+s.ID, "", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := decodeStream(t, rec); got.ListenerCount != 3 {
		t.Errorf("ListenerCount = %d, want 3", got.ListenerCount)
	}
}

func TestStreamHandler_GetUnknownStream(t *testing.T) {
	rig := newStreamTestRig(t)

	rec := rig.do(t, http.MethodGet, "/api/v1/streams/ghost", "", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStreamHandler_ListAndListLive(t *testing.T) {
	rig := newStreamTestRig(t)
	offline := rig.seed(t, "user-1", "Offline")
	live := rig.seed(t, "user-2", "Live")

	if err := rig.repo.UpdateStatus(context.Background(), live.ID, entity.StreamStatusLive); err != nil {
		t.Fatalf("update status: %v", err)
	}
	rig.registry.Open(context.Background(), live.ID)

	t.Run("list returns every stream", func(t *testing.T) {
		rec := rig.do(t, http.MethodGet, "/api/v1/streams", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var got []dto.StreamResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d streams, want 2", len(got))
		}
	})

	t.Run("live returns only broadcasting streams", func(t *testing.T) {
		rec := rig.do(t, http.MethodGet, "/api/v1/streams/live", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var got []dto.StreamResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d live streams, want 1", len(got))
		}
		if got[0].ID != live.ID {
			t.Errorf("live stream = %q, want %q (offline stream %q leaked)", got[0].ID, live.ID, offline.ID)
		}
	})

	t.Run("empty list serialises as [] not null", func(t *testing.T) {
		empty := newStreamTestRig(t)
		rec := empty.do(t, http.MethodGet, "/api/v1/streams", "", nil)
		if body := rec.Body.String(); body != "[]\n" {
			t.Errorf("body = %q, want %q — null breaks the Flutter decoder", body, "[]\n")
		}
	})
}

func TestStreamHandler_ListSurfacesRepositoryFailuresAs500(t *testing.T) {
	// A database outage must not look like "there are no streams" to the
	// mobile client — that would silently show an empty catalogue.
	rig := newStreamTestRig(t)
	rig.repo.failList = errors.New("connection refused")

	for _, path := range []string{"/api/v1/streams", "/api/v1/streams/live"} {
		t.Run(path, func(t *testing.T) {
			rec := rig.do(t, http.MethodGet, path, "", nil)
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
		})
	}
}

func TestStreamHandler_DeleteOwnership(t *testing.T) {
	rig := newStreamTestRig(t)
	s := rig.seed(t, "user-1", "Jazz")

	t.Run("a stranger gets 403", func(t *testing.T) {
		token := rig.token(t, "attacker", string(entity.RoleUser))
		rec := rig.do(t, http.MethodDelete, "/api/v1/streams/"+s.ID, token, nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("the owner gets 204", func(t *testing.T) {
		token := rig.token(t, "user-1", string(entity.RoleUser))
		rec := rig.do(t, http.MethodDelete, "/api/v1/streams/"+s.ID, token, nil)
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
	})

	t.Run("deleting an unknown stream gets 404", func(t *testing.T) {
		token := rig.token(t, "user-1", string(entity.RoleUser))
		rec := rig.do(t, http.MethodDelete, "/api/v1/streams/ghost", token, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func TestStreamHandler_ListenRejectsAnOfflineStream(t *testing.T) {
	rig := newStreamTestRig(t)
	s := rig.seed(t, "user-1", "Jazz")

	rec := rig.do(t, http.MethodGet, "/api/v1/streams/"+s.ID+"/listen", "", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStreamHandler_PublishRejectsNonOwners(t *testing.T) {
	rig := newStreamTestRig(t)
	s := rig.seed(t, "user-1", "Jazz")
	token := rig.token(t, "attacker", string(entity.RoleUser))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/"+s.ID+"/publish", bytes.NewBufferString("audio"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	rig.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rig.registry.IsLive(s.ID) {
		t.Error("a rejected publish still opened a hub")
	}
}

func TestStreamHandler_PublishMarksTheStreamOfflineWhenItEnds(t *testing.T) {
	rig := newStreamTestRig(t)
	s := rig.seed(t, "user-1", "Jazz")
	token := rig.token(t, "user-1", string(entity.RoleUser))

	// A finite body: the handler returns as soon as it hits EOF.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/"+s.ID+"/publish",
		bytes.NewBufferString("some audio bytes"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	rig.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rig.registry.IsLive(s.ID) {
		t.Error("the hub is still open after the broadcaster disconnected")
	}
	stored, err := rig.repo.FindByID(context.Background(), s.ID)
	if err != nil {
		t.Fatalf("find stream: %v", err)
	}
	if stored.Status != entity.StreamStatusOffline {
		t.Errorf("status = %q, want offline once publishing ended", stored.Status)
	}
}

// The wait before a listener hears anything is the quality metric the product
// is judged on, and it is not derivable from the HTTP duration histogram: a
// listen request lasts as long as the broadcast, so its duration measures the
// session, not the wait.
func TestStreamHandler_ListenRecordsTimeToFirstChunk(t *testing.T) {
	rig := newStreamTestRig(t)
	s := rig.seed(t, "user-1", "Jazz")

	hub := rig.registry.Open(context.Background(), s.ID)

	before := histogramCount(t, observability.ListenerTimeToFirstChunk)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/streams/"+s.ID+"/listen", nil)
		ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
		defer cancel()
		rig.mux.ServeHTTP(httptest.NewRecorder(), req.WithContext(ctx))
	}()

	// The listener attaches on its own goroutine, so publish until the
	// observation lands rather than guessing at a sleep.
	deadline := time.After(3 * time.Second)
	for histogramCount(t, observability.ListenerTimeToFirstChunk) == before {
		select {
		case <-deadline:
			t.Fatal("no observation recorded — the listener never received a chunk")
		default:
		}
		_ = hub.Publish([]byte("audio"))
		time.Sleep(5 * time.Millisecond)
	}

	rig.registry.Close(s.ID)
	<-done

	if got := histogramCount(t, observability.ListenerTimeToFirstChunk); got != before+1 {
		t.Errorf("observations = %d, want %d — exactly one per listener, not one per chunk", got, before+1)
	}
}

// histogramCount reads how many samples a histogram has observed so far.
func histogramCount(t *testing.T, h prometheus.Histogram) uint64 {
	t.Helper()
	var m clientmodel.Metric
	if err := h.Write(&m); err != nil {
		t.Fatalf("Write() = %v", err)
	}
	return m.GetHistogram().GetSampleCount()
}
