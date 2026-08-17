package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/infrastructure/streaming"
	"github.com/streampulse/backend/internal/transport/http/handler"
)

// These are end-to-end tests over a real HTTP server: a real broadcaster
// connection, real chunked responses, real listeners. Recorder-based tests
// cannot cover the streaming paths, because httptest.ResponseRecorder has no
// notion of a connection that stays open.

const streamTestSecret = "test-secret-at-least-32-bytes-long"

type memStreamRepo struct {
	mu   sync.Mutex
	byID map[string]*entity.Stream
}

func newMemStreamRepo() *memStreamRepo {
	return &memStreamRepo{byID: map[string]*entity.Stream{}}
}

func (r *memStreamRepo) Create(_ context.Context, s *entity.Stream) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *s
	r.byID[s.ID] = &clone
	return nil
}

func (r *memStreamRepo) FindByID(_ context.Context, id string) (*entity.Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byID[id]; ok {
		clone := *s
		return &clone, nil
	}
	return nil, repository.ErrNotFound
}

func (r *memStreamRepo) List(_ context.Context) ([]entity.Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Stream, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, *s)
	}
	return out, nil
}

func (r *memStreamRepo) ListByStatus(_ context.Context, status entity.StreamStatus) ([]entity.Stream, error) {
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

func (r *memStreamRepo) UpdateStatus(_ context.Context, id string, status entity.StreamStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	s.Status = status
	return nil
}

func (r *memStreamRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	return nil
}

var _ repository.StreamRepository = (*memStreamRepo)(nil)

type liveRig struct {
	server   *httptest.Server
	repo     *memStreamRepo
	registry *streaming.Registry
	jwt      *auth.JWTManager
	streamID string
	owner    string
}

func newLiveRig(t *testing.T) *liveRig {
	t.Helper()

	repo := newMemStreamRepo()
	registry := streaming.NewRegistry()
	uc := usecase.NewStreamUseCase(repo, registry)
	jwtManager := auth.NewJWTManager(streamTestSecret, time.Hour)

	mux := New(Handlers{
		Auth:   handler.NewAuthHandler(nil),
		Stream: handler.NewStreamHandler(uc),
	}, jwtManager)

	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		// Close the registry first: it releases the listener handlers still
		// parked on their hubs. srv.Close() waits for active connections, so
		// the reverse order deadlocks for 5s on every streaming test.
		registry.CloseAll()
		srv.Close()
	})

	rig := &liveRig{
		server: srv, repo: repo, registry: registry, jwt: jwtManager,
		streamID: "stream-1", owner: "user-1",
	}
	if err := repo.Create(context.Background(), &entity.Stream{
		ID: rig.streamID, Title: "Jazz de nuit", BroadcasterID: rig.owner,
		Status: entity.StreamStatusOffline,
	}); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	return rig
}

func (rig *liveRig) token(t *testing.T, userID string) string {
	t.Helper()
	tok, err := rig.jwt.Generate(userID, string(entity.RoleUser))
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

// startBroadcast opens a chunked publish connection and returns a writer for
// audio plus a stop function that ends the broadcast.
func (rig *liveRig) startBroadcast(t *testing.T, userID string) (io.Writer, func()) {
	t.Helper()

	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodPost,
		rig.server.URL+"/api/v1/streams/"+rig.streamID+"/publish", pr)
	if err != nil {
		t.Fatalf("build publish request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+rig.token(t, userID))
	// No ContentLength: net/http switches to chunked transfer encoding,
	// which is exactly how a real broadcaster streams.

	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	// Wait for the hub to exist, which proves the server accepted the publish.
	waitFor(t, 3*time.Second, func() bool { return rig.registry.IsLive(rig.streamID) })

	stop := func() {
		_ = pw.Close()
		select {
		case resp := <-respCh:
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		case err := <-errCh:
			t.Logf("publish request ended: %v", err)
		case <-time.After(3 * time.Second):
			t.Log("publish request did not finish within 3s")
		}
	}
	return pw, stop
}

// listen opens a listener connection and returns its response body.
func (rig *liveRig) listen(t *testing.T) io.ReadCloser {
	t.Helper()

	resp, err := http.Get(rig.server.URL + "/api/v1/streams/" + rig.streamID + "/listen") //nolint:noctx // test client
	if err != nil {
		t.Fatalf("listen request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("listen status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("Content-Type = %q, want audio/mpeg", ct)
	}
	return resp.Body
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within " + d.String())
}

// readExactly reads n bytes, giving up after 5s so a test never hangs on a
// stream that stopped producing. It returns an error rather than calling
// t.Fatal so it is safe to call from a goroutine.
func readExactly(r io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	done := make(chan error, 1)
	go func() { _, err := io.ReadFull(r, buf); done <- err }()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("read %d bytes: %w", n, err)
		}
		return buf, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timed out reading %d bytes from the stream", n)
	}
}

// mustReadExactly is the test-goroutine-only variant.
func mustReadExactly(t *testing.T, r io.Reader, n int) []byte {
	t.Helper()
	buf, err := readExactly(r, n)
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

func TestLiveStream_OneBroadcasterManySimultaneousListeners(t *testing.T) {
	// The K1 acceptance test: audio published once reaches every listener.
	const listeners = 25

	rig := newLiveRig(t)
	audio, stopBroadcast := rig.startBroadcast(t, rig.owner)
	defer stopBroadcast()

	bodies := make([]io.ReadCloser, listeners)
	for i := range listeners {
		bodies[i] = rig.listen(t)
		defer func(b io.ReadCloser) { _ = b.Close() }(bodies[i])
	}
	waitFor(t, 3*time.Second, func() bool {
		return rig.registry.ListenerCount(rig.streamID) == listeners
	})

	const payload = "AUDIO-CHUNK-0123456789"
	if _, err := audio.Write([]byte(payload)); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	// Read every listener concurrently: sequentially would let an early
	// listener's buffer drain while a later one stalls, hiding a fan-out bug.
	var wg sync.WaitGroup
	got := make([]string, listeners)
	errs := make([]error, listeners)
	for i := range listeners {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			chunk, err := readExactly(bodies[i], len(payload))
			got[i], errs[i] = string(chunk), err
		}(i)
	}
	wg.Wait()

	for i := range listeners {
		if errs[i] != nil {
			t.Errorf("listener %d: %v", i, errs[i])
			continue
		}
		if got[i] != payload {
			t.Errorf("listener %d received %q, want %q", i, got[i], payload)
		}
	}
}

func TestLiveStream_ListenerJoiningLateReceivesSubsequentAudio(t *testing.T) {
	rig := newLiveRig(t)
	audio, stopBroadcast := rig.startBroadcast(t, rig.owner)
	defer stopBroadcast()

	// Audio published before anyone is listening is simply gone — this is a
	// live stream, not a recording. The late listener picks up from "now".
	if _, err := audio.Write([]byte("MISSED")); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	body := rig.listen(t)
	defer func() { _ = body.Close() }()
	waitFor(t, 3*time.Second, func() bool { return rig.registry.ListenerCount(rig.streamID) == 1 })

	if _, err := audio.Write([]byte("HEARD!")); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	if got := string(mustReadExactly(t, body, 6)); got != "HEARD!" {
		t.Errorf("late listener received %q, want %q", got, "HEARD!")
	}
}

func TestLiveStream_StoppingTheBroadcastEndsListenerConnections(t *testing.T) {
	rig := newLiveRig(t)
	audio, stopBroadcast := rig.startBroadcast(t, rig.owner)

	body := rig.listen(t)
	defer func() { _ = body.Close() }()
	waitFor(t, 3*time.Second, func() bool { return rig.registry.ListenerCount(rig.streamID) == 1 })

	if _, err := audio.Write([]byte("bye")); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	mustReadExactly(t, body, 3)

	stopBroadcast()

	// The listener's response body must reach EOF rather than hang forever.
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, body)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("listener body ended with %v, want a clean EOF", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener connection did not close when the broadcast stopped")
	}

	// And the stream is back to offline for everyone browsing.
	waitFor(t, 3*time.Second, func() bool { return !rig.registry.IsLive(rig.streamID) })
	stored, err := rig.repo.FindByID(context.Background(), rig.streamID)
	if err != nil {
		t.Fatalf("find stream: %v", err)
	}
	if stored.Status != entity.StreamStatusOffline {
		t.Errorf("status = %q, want offline", stored.Status)
	}
}

func TestLiveStream_AListenerLeavingDoesNotDisturbTheOthers(t *testing.T) {
	rig := newLiveRig(t)
	audio, stopBroadcast := rig.startBroadcast(t, rig.owner)
	defer stopBroadcast()

	leaving := rig.listen(t)
	staying := rig.listen(t)
	defer func() { _ = staying.Close() }()
	waitFor(t, 3*time.Second, func() bool { return rig.registry.ListenerCount(rig.streamID) == 2 })

	_ = leaving.Close()
	waitFor(t, 3*time.Second, func() bool { return rig.registry.ListenerCount(rig.streamID) == 1 })

	if _, err := audio.Write([]byte("STILL-HERE")); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	if got := string(mustReadExactly(t, staying, 10)); got != "STILL-HERE" {
		t.Errorf("remaining listener received %q, want %q", got, "STILL-HERE")
	}
}

func TestLiveStream_PublishOverWebSocket(t *testing.T) {
	// Browsers cannot stream an HTTP request body, so the web broadcaster
	// pushes binary frames over a WebSocket instead. Same hub, same listeners.
	rig := newLiveRig(t)

	wsURL := "ws" + strings.TrimPrefix(rig.server.URL, "http") +
		"/api/v1/streams/" + rig.streamID + "/publish/ws" +
		"?token=" + url.QueryEscape(rig.token(t, rig.owner))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	waitFor(t, 3*time.Second, func() bool { return rig.registry.IsLive(rig.streamID) })

	body := rig.listen(t)
	defer func() { _ = body.Close() }()
	waitFor(t, 3*time.Second, func() bool { return rig.registry.ListenerCount(rig.streamID) == 1 })

	if err := conn.Write(ctx, websocket.MessageBinary, []byte("WS-AUDIO")); err != nil {
		t.Fatalf("websocket write: %v", err)
	}

	if got := string(mustReadExactly(t, body, 8)); got != "WS-AUDIO" {
		t.Errorf("listener received %q, want %q", got, "WS-AUDIO")
	}
}

func TestLiveStream_WebSocketPublishRejectsABadToken(t *testing.T) {
	rig := newLiveRig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	base := "ws" + strings.TrimPrefix(rig.server.URL, "http") +
		"/api/v1/streams/" + rig.streamID + "/publish/ws"

	for name, target := range map[string]string{
		"no token":      base,
		"garbage token": base + "?token=not-a-jwt",
	} {
		t.Run(name, func(t *testing.T) {
			conn, resp, err := websocket.Dial(ctx, target, nil)
			if err == nil {
				_ = conn.CloseNow()
				t.Fatal("dial succeeded, want it rejected")
			}
			if resp != nil && resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
			if rig.registry.IsLive(rig.streamID) {
				t.Error("a rejected upgrade still opened a hub")
			}
		})
	}
}

func TestLiveStream_WebSocketPublishRejectsNonOwners(t *testing.T) {
	rig := newLiveRig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(rig.server.URL, "http") +
		"/api/v1/streams/" + rig.streamID + "/publish/ws" +
		"?token=" + url.QueryEscape(rig.token(t, "attacker"))

	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		_ = conn.CloseNow()
		t.Fatal("dial succeeded for a non-owner, want it rejected")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestRouter_StreamRouteAuthMatrix(t *testing.T) {
	rig := newLiveRig(t)

	cases := []struct {
		name       string
		method     string
		path       string
		wantPublic bool
	}{
		{"browse all", http.MethodGet, "/api/v1/streams", true},
		{"browse live", http.MethodGet, "/api/v1/streams/live", true},
		{"stream detail", http.MethodGet, "/api/v1/streams/" + rig.streamID, true},
		{"listen", http.MethodGet, "/api/v1/streams/" + rig.streamID + "/listen", true},
		{"create", http.MethodPost, "/api/v1/streams", false},
		{"delete", http.MethodDelete, "/api/v1/streams/" + rig.streamID, false},
		{"publish", http.MethodPost, "/api/v1/streams/" + rig.streamID + "/publish", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			New(Handlers{
				Auth: handler.NewAuthHandler(nil),
				Stream: handler.NewStreamHandler(
					usecase.NewStreamUseCase(rig.repo, rig.registry)),
			}, rig.jwt).ServeHTTP(rec, req)

			gotUnauthorized := rec.Code == http.StatusUnauthorized
			if tc.wantPublic && gotUnauthorized {
				t.Errorf("%s %s returned 401, want it public", tc.method, tc.path)
			}
			if !tc.wantPublic && !gotUnauthorized {
				t.Errorf("%s %s returned %d, want 401 without a token", tc.method, tc.path, rec.Code)
			}
		})
	}
}

func TestRouter_ListenIsNotCached(t *testing.T) {
	// A cached audio stream would replay stale audio to the next listener.
	rig := newLiveRig(t)
	_, stopBroadcast := rig.startBroadcast(t, rig.owner)
	defer stopBroadcast()

	resp, err := http.Get(rig.server.URL + "/api/v1/streams/" + rig.streamID + "/listen") //nolint:noctx // test client
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no (proxies would buffer the audio)", got)
	}
}

func TestRouter_StreamJSONShape(t *testing.T) {
	// The Flutter client decodes these keys; renaming one silently breaks the
	// app, so the contract is pinned here.
	rig := newLiveRig(t)

	resp, err := http.Get(rig.server.URL + "/api/v1/streams/" + rig.streamID) //nolint:noctx // test client
	if err != nil {
		t.Fatalf("get stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"id", "title", "description", "broadcaster_id", "status", "listener_count", "created_at", "updated_at"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("response is missing the %q field", key)
		}
	}
}
