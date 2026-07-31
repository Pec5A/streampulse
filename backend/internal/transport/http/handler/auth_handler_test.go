package handler

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
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

// fakeUserRepo is a minimal in-memory repository.UserRepository for
// handler-level tests (a real HTTP round trip through the usecase, not a
// mocked usecase — catches wiring/status-code mistakes a mock would hide).
type fakeUserRepo struct {
	mu      sync.Mutex
	byID    map[string]*entity.User
	byEmail map[string]string
	counter int
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[string]*entity.User{}, byEmail: map[string]string{}}
}
func (r *fakeUserRepo) Create(_ context.Context, u *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counter++
	if u.ID == "" {
		u.ID = "user-" + string(rune('a'-1+r.counter))
	}
	r.byID[u.ID] = u
	r.byEmail[u.Email] = u.ID
	return nil
}
func (r *fakeUserRepo) FindByID(_ context.Context, id string) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		return u, nil
	}
	return nil, repository.ErrNotFound
}
func (r *fakeUserRepo) FindByEmail(_ context.Context, email string) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.byEmail[email]; ok {
		return r.byID[id], nil
	}
	return nil, repository.ErrNotFound
}
func (r *fakeUserRepo) Update(_ context.Context, u *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[u.ID] = u
	return nil
}
func (r *fakeUserRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	return nil
}

func newTestHandler() (*AuthHandler, *auth.JWTManager) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	uc := usecase.NewAuthUseCase(newFakeUserRepo(), jwtManager, auth.NewBcryptHasher())
	return NewAuthHandler(uc), jwtManager
}

func doJSON(h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestAuthHandler_Register_Success(t *testing.T) {
	h, _ := newTestHandler()
	rec := doJSON(h.Register, http.MethodPost, `{"email":"a@b.com","username":"alice","password":"hunter2222"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp dto.AuthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.User.Email != "a@b.com" || resp.Token == "" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestAuthHandler_Register_InvalidBody(t *testing.T) {
	h, _ := newTestHandler()
	rec := doJSON(h.Register, http.MethodPost, `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthHandler_Register_ShortPassword(t *testing.T) {
	h, _ := newTestHandler()
	rec := doJSON(h.Register, http.MethodPost, `{"email":"a@b.com","username":"alice","password":"short"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthHandler_Register_DuplicateEmailReturns409(t *testing.T) {
	h, _ := newTestHandler()
	doJSON(h.Register, http.MethodPost, `{"email":"dup@b.com","username":"first","password":"hunter2222"}`)
	rec := doJSON(h.Register, http.MethodPost, `{"email":"dup@b.com","username":"second","password":"hunter2222"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestAuthHandler_Login_Success(t *testing.T) {
	h, _ := newTestHandler()
	doJSON(h.Register, http.MethodPost, `{"email":"login@b.com","username":"loginer","password":"hunter2222"}`)

	rec := doJSON(h.Login, http.MethodPost, `{"email":"login@b.com","password":"hunter2222"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestAuthHandler_Login_WrongPasswordReturns401(t *testing.T) {
	h, _ := newTestHandler()
	doJSON(h.Register, http.MethodPost, `{"email":"wp@b.com","username":"wp","password":"hunter2222"}`)

	rec := doJSON(h.Login, http.MethodPost, `{"email":"wp@b.com","password":"wrong-one"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthHandler_Refresh_RequiresAuth(t *testing.T) {
	h, jwtManager := newTestHandler()

	regRec := doJSON(h.Register, http.MethodPost, `{"email":"ref@b.com","username":"ref","password":"hunter2222"}`)
	var reg dto.AuthResponse
	if err := json.Unmarshal(regRec.Body.Bytes(), &reg); err != nil {
		t.Fatalf("decode register response: %v", err)
	}

	// Wrap Refresh with the real middleware, exactly as the router will.
	protected := middleware.RequireAuth(jwtManager)(http.HandlerFunc(h.Refresh))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer "+reg.Token)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	noAuthReq := httptest.NewRequest(http.MethodPost, "/", nil)
	noAuthRec := httptest.NewRecorder()
	protected.ServeHTTP(noAuthRec, noAuthReq)
	if noAuthRec.Code != http.StatusUnauthorized {
		t.Fatalf("without token: status = %d, want %d", noAuthRec.Code, http.StatusUnauthorized)
	}
}
