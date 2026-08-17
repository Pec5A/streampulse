package repository

import (
	"context"

	"github.com/streampulse/backend/internal/domain/entity"
)

// TrackRepository persists uploaded-track metadata. The bytes themselves
// live behind the storage port, never in the database.
type TrackRepository interface {
	Create(ctx context.Context, track *entity.Track) error
	FindByID(ctx context.Context, id string) (*entity.Track, error)
	List(ctx context.Context, limit, offset int) ([]entity.Track, error)
	ListByUploader(ctx context.Context, uploaderID string) ([]entity.Track, error)
	Delete(ctx context.Context, id string) error
}
