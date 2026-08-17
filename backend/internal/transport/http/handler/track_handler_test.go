package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/application/dto"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/infrastructure/storage"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

// trackTestRig wires the real use case over a real on-disk storage in a temp
// directory, so uploads exercise the actual write/serve path.
type trackTestRig struct {
	mux  *http.ServeMux
	jwt  *auth.JWTManager
	repo *stubTrackRepo
}

func newTrackTestRig(t *testing.T) *trackTestRig {
	t.Helper()

	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	repo := newStubTrackRepo()
	uc := usecase.NewTrackUseCase(repo, store)
	h := NewTrackHandler(uc)
	jwtManager := auth.NewJWTManager(testSecret, time.Hour)
	authed := middleware.RequireAuth(jwtManager)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tracks", h.List)
	mux.HandleFunc("GET /api/v1/tracks/{id}", h.Get)
	mux.HandleFunc("GET /api/v1/tracks/{id}/audio", h.Audio)
	mux.Handle("POST /api/v1/tracks", authed(http.HandlerFunc(h.Upload)))
	mux.Handle("GET /api/v1/tracks/mine", authed(http.HandlerFunc(h.ListMine)))
	mux.Handle("DELETE /api/v1/tracks/{id}", authed(http.HandlerFunc(h.Delete)))

	return &trackTestRig{mux: mux, jwt: jwtManager, repo: repo}
}

func (rig *trackTestRig) token(t *testing.T, userID, role string) string {
	t.Helper()
	tok, err := rig.jwt.Generate(userID, role)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

// multipartUpload builds a multipart body the way a real client would,
// including the per-part Content-Type the server validates.
func multipartUpload(t *testing.T, title, artist, filename, contentType string, content []byte) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if title != "" {
		if err := w.WriteField("title", title); err != nil {
			t.Fatalf("write title: %v", err)
		}
	}
	if artist != "" {
		if err := w.WriteField("artist", artist); err != nil {
			t.Fatalf("write artist: %v", err)
		}
	}
	if filename != "" {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
		h.Set("Content-Type", contentType)
		part, err := w.CreatePart(h)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

// audioBytes returns a payload the sniffer sees as audio/mpeg.
func audioBytes(size int) []byte {
	b := make([]byte, size)
	copy(b, "ID3\x03\x00\x00\x00")
	for i := 32; i < size; i++ {
		b[i] = byte(i % 251)
	}
	return b
}

func (rig *trackTestRig) upload(t *testing.T, token string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tracks", body)
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	rig.mux.ServeHTTP(rec, req)
	return rec
}

func TestTrackHandler_UploadRequiresAuth(t *testing.T) {
	rig := newTrackTestRig(t)
	body, ct := multipartUpload(t, "Nocturne", "", "n.mp3", "audio/mpeg", audioBytes(1024))

	rec := rig.upload(t, "", body, ct)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestTrackHandler_Upload(t *testing.T) {
	rig := newTrackTestRig(t)
	token := rig.token(t, "user-1", string(entity.RoleBroadcaster))
	body, ct := multipartUpload(t, "Nocturne", "KaysZ", "nocturne.mp3", "audio/mpeg", audioBytes(4096))

	rec := rig.upload(t, token, body, ct)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body)
	}
	var got dto.AudioTrackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "Nocturne" || got.Artist != "KaysZ" {
		t.Errorf("got %+v, unexpected metadata", got)
	}
	if got.SizeBytes != 4096 {
		t.Errorf("SizeBytes = %d, want 4096", got.SizeBytes)
	}
	if got.UploaderID != "user-1" {
		t.Errorf("UploaderID = %q, want user-1", got.UploaderID)
	}
	if got.AudioURL != "/api/v1/tracks/"+got.ID+"/audio" {
		t.Errorf("AudioURL = %q, unexpected", got.AudioURL)
	}
	if strings.Contains(rec.Body.String(), "storage_key") {
		t.Error("the response leaked the internal storage key")
	}
}

func TestTrackHandler_UploadRejectsHTMLDisguisedAsAudio(t *testing.T) {
	rig := newTrackTestRig(t)
	token := rig.token(t, "user-1", string(entity.RoleUser))
	html := []byte("<!DOCTYPE html><html><body><script>alert(1)</script></body></html>")
	body, ct := multipartUpload(t, "Nocturne", "", "innocent.mp3", "audio/mpeg", html)

	rec := rig.upload(t, token, body, ct)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
}

func TestTrackHandler_UploadRejectsAMissingFilePart(t *testing.T) {
	rig := newTrackTestRig(t)
	token := rig.token(t, "user-1", string(entity.RoleUser))
	body, ct := multipartUpload(t, "Nocturne", "", "", "", nil)

	rec := rig.upload(t, token, body, ct)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTrackHandler_UploadRejectsAMissingTitle(t *testing.T) {
	rig := newTrackTestRig(t)
	token := rig.token(t, "user-1", string(entity.RoleUser))
	body, ct := multipartUpload(t, "", "", "n.mp3", "audio/mpeg", audioBytes(1024))

	rec := rig.upload(t, token, body, ct)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTrackHandler_UploadRejectsAnOversizedBody(t *testing.T) {
	// The cap is applied by MaxBytesReader while the body streams, so the
	// server never has to buffer the whole oversized upload to refuse it.
	rig := newTrackTestRig(t)
	token := rig.token(t, "user-1", string(entity.RoleUser))
	huge := audioBytes(usecase.MaxUploadBytes + multipartOverhead + 4096)
	body, ct := multipartUpload(t, "Nocturne", "", "big.mp3", "audio/mpeg", huge)

	rec := rig.upload(t, token, body, ct)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestTrackHandler_UploadRejectsANonAudioContentType(t *testing.T) {
	rig := newTrackTestRig(t)
	token := rig.token(t, "user-1", string(entity.RoleUser))
	body, ct := multipartUpload(t, "Nocturne", "", "n.png", "image/png", audioBytes(1024))

	rec := rig.upload(t, token, body, ct)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
}

// uploadTrack posts a track and returns its decoded response.
func (rig *trackTestRig) uploadTrack(t *testing.T, userID string, content []byte) dto.AudioTrackResponse {
	t.Helper()
	token := rig.token(t, userID, string(entity.RoleUser))
	body, ct := multipartUpload(t, "Nocturne", "KaysZ", "nocturne.mp3", "audio/mpeg", content)
	rec := rig.upload(t, token, body, ct)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201 (body: %s)", rec.Code, rec.Body)
	}
	var got dto.AudioTrackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func TestTrackHandler_AudioServesTheFileWithSafeHeaders(t *testing.T) {
	rig := newTrackTestRig(t)
	content := audioBytes(2048)
	track := rig.uploadTrack(t, "user-1", content)

	req := httptest.NewRequest(http.MethodGet, track.AudioURL, nil)
	rec := httptest.NewRecorder()
	rig.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !bytes.Equal(rec.Body.Bytes(), content) {
		t.Error("served bytes differ from the uploaded ones")
	}
	// The declared type we validated, never a sniffed one — and the browser
	// is told not to second-guess it.
	if got := rec.Header().Get("Content-Type"); got != "audio/mpeg" {
		t.Errorf("Content-Type = %q, want audio/mpeg", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("no Content-Security-Policy on served user content")
	}
}

func TestTrackHandler_AudioSupportsRangeRequests(t *testing.T) {
	// This is what makes seeking work in the mobile player: without Range
	// support it would have to re-download the track from byte zero.
	rig := newTrackTestRig(t)
	content := audioBytes(2048)
	track := rig.uploadTrack(t, "user-1", content)

	req := httptest.NewRequest(http.MethodGet, track.AudioURL, nil)
	req.Header.Set("Range", "bytes=1000-1099")
	rec := httptest.NewRecorder()
	rig.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusPartialContent)
	}
	if got := rec.Body.Len(); got != 100 {
		t.Errorf("served %d bytes, want 100", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), content[1000:1100]) {
		t.Error("the served range does not match the requested bytes")
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 1000-1099/2048" {
		t.Errorf("Content-Range = %q, want bytes 1000-1099/2048", got)
	}
}

func TestTrackHandler_AudioUnknownTrack(t *testing.T) {
	rig := newTrackTestRig(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tracks/ghost/audio", nil)
	rec := httptest.NewRecorder()
	rig.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestTrackHandler_ListAndListMine(t *testing.T) {
	rig := newTrackTestRig(t)
	rig.uploadTrack(t, "user-1", audioBytes(512))
	rig.uploadTrack(t, "user-2", audioBytes(512))

	t.Run("the catalogue is public and lists everything", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tracks", nil)
		rec := httptest.NewRecorder()
		rig.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var got []dto.AudioTrackResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d tracks, want 2", len(got))
		}
	})

	t.Run("mine lists only the caller's uploads", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tracks/mine", nil)
		req.Header.Set("Authorization", "Bearer "+rig.token(t, "user-1", string(entity.RoleUser)))
		rec := httptest.NewRecorder()
		rig.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var got []dto.AudioTrackResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got) != 1 || got[0].UploaderID != "user-1" {
			t.Errorf("got %+v, want only user-1's track", got)
		}
	})

	t.Run("mine requires auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tracks/mine", nil)
		rec := httptest.NewRecorder()
		rig.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("an empty catalogue serialises as [] not null", func(t *testing.T) {
		empty := newTrackTestRig(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tracks", nil)
		rec := httptest.NewRecorder()
		empty.mux.ServeHTTP(rec, req)
		if body := rec.Body.String(); body != "[]\n" {
			t.Errorf("body = %q, want %q", body, "[]\n")
		}
	})
}

func TestTrackHandler_Delete(t *testing.T) {
	rig := newTrackTestRig(t)
	track := rig.uploadTrack(t, "user-1", audioBytes(512))

	t.Run("a stranger gets 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/tracks/"+track.ID, nil)
		req.Header.Set("Authorization", "Bearer "+rig.token(t, "attacker", string(entity.RoleUser)))
		rec := httptest.NewRecorder()
		rig.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("the uploader gets 204 and the audio is gone", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/tracks/"+track.ID, nil)
		req.Header.Set("Authorization", "Bearer "+rig.token(t, "user-1", string(entity.RoleUser)))
		rec := httptest.NewRecorder()
		rig.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}

		audioReq := httptest.NewRequest(http.MethodGet, track.AudioURL, nil)
		audioRec := httptest.NewRecorder()
		rig.mux.ServeHTTP(audioRec, audioReq)
		if audioRec.Code != http.StatusNotFound {
			t.Errorf("audio still served after delete: status = %d", audioRec.Code)
		}
	})
}

func TestTrackHandler_Get(t *testing.T) {
	rig := newTrackTestRig(t)
	track := rig.uploadTrack(t, "user-1", audioBytes(512))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tracks/"+track.ID, nil)
	rec := httptest.NewRecorder()
	rig.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got dto.AudioTrackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != track.ID {
		t.Errorf("id = %q, want %q", got.ID, track.ID)
	}
}

// stubTrackRepo mirrors fakeTrackRepo but lives in the handler package.
type stubTrackRepo struct {
	byID    map[string]*entity.AudioTrack
	counter int
}

func newStubTrackRepo() *stubTrackRepo {
	return &stubTrackRepo{byID: map[string]*entity.AudioTrack{}}
}

func (r *stubTrackRepo) Create(_ context.Context, t *entity.AudioTrack) error {
	r.counter++
	if t.ID == "" {
		t.ID = "track-" + string(rune('a'-1+r.counter))
	}
	clone := *t
	r.byID[t.ID] = &clone
	return nil
}

func (r *stubTrackRepo) FindByID(_ context.Context, id string) (*entity.AudioTrack, error) {
	if t, ok := r.byID[id]; ok {
		clone := *t
		return &clone, nil
	}
	return nil, repository.ErrNotFound
}

func (r *stubTrackRepo) List(_ context.Context, limit, offset int) ([]entity.AudioTrack, error) {
	out := make([]entity.AudioTrack, 0, len(r.byID))
	for _, t := range r.byID {
		out = append(out, *t)
	}
	if offset >= len(out) {
		return []entity.AudioTrack{}, nil
	}
	return out[offset:min(offset+limit, len(out))], nil
}

func (r *stubTrackRepo) ListByUploader(_ context.Context, uploaderID string) ([]entity.AudioTrack, error) {
	out := make([]entity.AudioTrack, 0)
	for _, t := range r.byID {
		if t.UploaderID == uploaderID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (r *stubTrackRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.byID[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.byID, id)
	return nil
}
