package usecase

import (
	"context"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

type UserUseCase struct {
	users repository.UserRepository
}

func NewUserUseCase(users repository.UserRepository) *UserUseCase {
	return &UserUseCase{users: users}
}

func (uc *UserUseCase) Me(ctx context.Context, id string) (*entity.User, error) {
	return uc.users.FindByID(ctx, id)
}

// PersonalDataExport implements the RGPD right of access (article 15).
//
// Playlists and streams are not yet part of this export: those entities
// don't exist in this repo yet (tickets K1/S1 build them). Extend this
// struct and ExportData once they land — see docs/team/plan.md.
type PersonalDataExport struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

func (uc *UserUseCase) ExportData(ctx context.Context, id string) (*PersonalDataExport, error) {
	u, err := uc.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &PersonalDataExport{
		ID:        u.ID,
		Email:     u.Email,
		Username:  u.Username,
		Role:      string(u.Role),
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// DeleteMe implements the RGPD right to erasure (article 17).
//
// This only deletes the users row for now (no dependent tables exist yet
// in this repo). Once playlists/streams/tracks land (tickets K1/S1/K2),
// this must cascade to them too — either via ON DELETE CASCADE at the
// schema level, or an explicit transaction here; whichever that ticket's
// owner and this one agree on.
func (uc *UserUseCase) DeleteMe(ctx context.Context, id string) error {
	return uc.users.Delete(ctx, id)
}
