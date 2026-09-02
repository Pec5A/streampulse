package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

func TestUserUseCase_Me(t *testing.T) {
	repo := newFakeUserRepo()
	if err := repo.Create(context.Background(), &entity.User{Email: "a@b.com", Username: "a"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var id string
	for k := range repo.byID {
		id = k
	}
	uc := NewUserUseCase(repo)

	u, err := uc.Me(context.Background(), id)
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if u.Username != "a" {
		t.Errorf("Username = %q, want a", u.Username)
	}
}

func TestUserUseCase_Me_NotFound(t *testing.T) {
	uc := NewUserUseCase(newFakeUserRepo())
	if _, err := uc.Me(context.Background(), "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Me() error = %v, want repository.ErrNotFound", err)
	}
}

func TestUserUseCase_ExportData_SanitizesPasswordAndFillsFields(t *testing.T) {
	repo := newFakeUserRepo()
	if err := repo.Create(context.Background(), &entity.User{
		Email:    "export@b.com",
		Username: "exporter",
		Password: "should-never-appear-in-export",
		Role:     entity.RoleUser,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var id string
	for k := range repo.byID {
		id = k
	}
	uc := NewUserUseCase(repo)

	export, err := uc.ExportData(context.Background(), id)
	if err != nil {
		t.Fatalf("ExportData() error = %v", err)
	}
	if export.Email != "export@b.com" || export.Username != "exporter" || export.Role != "user" {
		t.Errorf("ExportData() = %+v, unexpected fields", export)
	}
	// PersonalDataExport has no Password field at all — the type itself
	// makes leaking the hash impossible, not just a runtime check.
}

func TestUserUseCase_ExportData_NotFound(t *testing.T) {
	uc := NewUserUseCase(newFakeUserRepo())
	if _, err := uc.ExportData(context.Background(), "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("ExportData() error = %v, want repository.ErrNotFound", err)
	}
}

func TestUserUseCase_DeleteMe(t *testing.T) {
	repo := newFakeUserRepo()
	if err := repo.Create(context.Background(), &entity.User{Email: "gone@b.com", Username: "gone"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var id string
	for k := range repo.byID {
		id = k
	}
	uc := NewUserUseCase(repo)

	if err := uc.DeleteMe(context.Background(), id); err != nil {
		t.Fatalf("DeleteMe() error = %v", err)
	}
	if _, err := uc.Me(context.Background(), id); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Me() after DeleteMe error = %v, want repository.ErrNotFound", err)
	}
}

func TestUserUseCase_DeleteMe_NotFound(t *testing.T) {
	uc := NewUserUseCase(newFakeUserRepo())
	if err := uc.DeleteMe(context.Background(), "missing"); err != nil {
		t.Fatalf("DeleteMe() on missing user error = %v, want nil (fakeUserRepo.Delete is idempotent)", err)
	}
}
