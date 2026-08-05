package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

type fakeAdminRepo struct {
	mu    sync.Mutex
	users map[string]*entity.User
}

func newFakeAdminRepo(users ...*entity.User) *fakeAdminRepo {
	m := make(map[string]*entity.User, len(users))
	for _, u := range users {
		cp := *u
		m[u.ID] = &cp
	}
	return &fakeAdminRepo{users: m}
}

func (r *fakeAdminRepo) ListUsers(_ context.Context, _, _ int) ([]entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, *u)
	}
	return out, nil
}

func (r *fakeAdminRepo) CountUsersByRole(_ context.Context) (map[entity.Role]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[entity.Role]int{}
	for _, u := range r.users {
		out[u.Role]++
	}
	return out, nil
}

func (r *fakeAdminRepo) FindByID(_ context.Context, id string) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, repository.ErrNotFound
}

func (r *fakeAdminRepo) Update(_ context.Context, u *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *u
	r.users[u.ID] = &cp
	return nil
}

var _ repository.AdminUserRepository = (*fakeAdminRepo)(nil)

func TestAdminUseCase_Stats(t *testing.T) {
	uc := NewAdminUseCase(newFakeAdminRepo(
		&entity.User{ID: "1", Role: entity.RoleUser},
		&entity.User{ID: "2", Role: entity.RoleUser},
		&entity.User{ID: "3", Role: entity.RoleBroadcaster},
		&entity.User{ID: "4", Role: entity.RoleAdmin},
	))
	s, err := uc.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if s.TotalUsers != 4 || s.RegularUsers != 2 || s.Broadcasters != 1 || s.Admins != 1 {
		t.Fatalf("Stats = %+v, want total4 reg2 broad1 admin1", s)
	}
}

func TestAdminUseCase_ListUsers(t *testing.T) {
	uc := NewAdminUseCase(newFakeAdminRepo(
		&entity.User{ID: "1", Role: entity.RoleUser},
		&entity.User{ID: "2", Role: entity.RoleAdmin},
	))
	users, err := uc.ListUsers(context.Background(), 0, 50)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len = %d, want 2", len(users))
	}
}

func TestAdminUseCase_UpdateRole_Success(t *testing.T) {
	uc := NewAdminUseCase(newFakeAdminRepo(&entity.User{ID: "1", Role: entity.RoleUser}))
	u, err := uc.UpdateRole(context.Background(), "1", "broadcaster")
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}
	if u.Role != entity.RoleBroadcaster {
		t.Errorf("role = %q, want broadcaster", u.Role)
	}
}

func TestAdminUseCase_UpdateRole_Invalid(t *testing.T) {
	uc := NewAdminUseCase(newFakeAdminRepo(&entity.User{ID: "1", Role: entity.RoleUser}))
	if _, err := uc.UpdateRole(context.Background(), "1", "superuser"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("error = %v, want ErrInvalidRole", err)
	}
}

func TestAdminUseCase_UpdateRole_NotFound(t *testing.T) {
	uc := NewAdminUseCase(newFakeAdminRepo())
	if _, err := uc.UpdateRole(context.Background(), "missing", "admin"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("error = %v, want repository.ErrNotFound", err)
	}
}
