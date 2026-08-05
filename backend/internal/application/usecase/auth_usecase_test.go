package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/auth"
)

// fakeUserRepo is an in-memory repository.UserRepository, sufficient for
// usecase-level testing.
type fakeUserRepo struct {
	mu      sync.Mutex
	byID    map[string]*entity.User
	byEmail map[string]string // email -> id
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
	if u, ok := r.byID[id]; ok {
		delete(r.byEmail, u.Email)
	}
	delete(r.byID, id)
	return nil
}

func newTestAuthUseCase(repo repository.UserRepository) *AuthUseCase {
	return NewAuthUseCase(repo, auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour), auth.NewBcryptHasher())
}

func TestAuthUseCase_Register_Success(t *testing.T) {
	uc := newTestAuthUseCase(newFakeUserRepo())

	user, token, err := uc.Register(context.Background(), "Alice@Example.com", " alice ", "hunter22")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("Email = %q, want lowercased+trimmed", user.Email)
	}
	if user.Username != "alice" {
		t.Errorf("Username = %q, want trimmed", user.Username)
	}
	if user.Role != entity.RoleUser {
		t.Errorf("Role = %q, want %q", user.Role, entity.RoleUser)
	}
	if user.Password == "hunter22" {
		t.Error("password stored in plaintext, expected it to be hashed")
	}
	if token == "" {
		t.Error("expected a non-empty JWT")
	}
}

func TestAuthUseCase_Register_DuplicateEmail(t *testing.T) {
	uc := newTestAuthUseCase(newFakeUserRepo())

	if _, _, err := uc.Register(context.Background(), "bob@example.com", "bob", "pw"); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	_, _, err := uc.Register(context.Background(), "bob@example.com", "bob2", "pw2")
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Register() error = %v, want repository.ErrConflict", err)
	}
}

func TestAuthUseCase_Login_Success(t *testing.T) {
	uc := newTestAuthUseCase(newFakeUserRepo())

	if _, _, err := uc.Register(context.Background(), "dave@example.com", "dave", "correct-horse"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	user, token, err := uc.Login(context.Background(), "DAVE@example.com", "correct-horse")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if user.Username != "dave" {
		t.Errorf("Username = %q, want dave", user.Username)
	}
	if token == "" {
		t.Error("expected a non-empty JWT")
	}
}

func TestAuthUseCase_Login_UserNotFound(t *testing.T) {
	uc := newTestAuthUseCase(newFakeUserRepo())

	_, _, err := uc.Login(context.Background(), "ghost@example.com", "whatever")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthUseCase_Login_WrongPassword(t *testing.T) {
	uc := newTestAuthUseCase(newFakeUserRepo())

	if _, _, err := uc.Register(context.Background(), "eve@example.com", "eve", "right-password"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_, _, err := uc.Login(context.Background(), "eve@example.com", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthUseCase_Refresh_Success(t *testing.T) {
	uc := newTestAuthUseCase(newFakeUserRepo())

	registered, _, err := uc.Register(context.Background(), "frank@example.com", "frank", "pw")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	user, token, err := uc.Refresh(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if user.ID != registered.ID {
		t.Errorf("Refresh() user.ID = %q, want %q", user.ID, registered.ID)
	}
	if token == "" {
		t.Error("expected a non-empty JWT")
	}
}

func TestAuthUseCase_Refresh_UserNotFound(t *testing.T) {
	uc := newTestAuthUseCase(newFakeUserRepo())

	_, _, err := uc.Refresh(context.Background(), "missing-id")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Refresh() error = %v, want repository.ErrNotFound", err)
	}
}
