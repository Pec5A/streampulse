package repository

import (
	"context"

	"github.com/streampulse/backend/internal/domain/entity"
)

// TrackRepository persists uploaded-track metadata. The bytes themselves
// live behind the storage port, never in the database.
type TrackRepository interface {
	Create(ctx context.Context, track *entity.AudioTrack) error
	FindByID(ctx context.Context, id string) (*entity.AudioTrack, error)
	List(ctx context.Context, limit, offset int) ([]entity.AudioTrack, error)
	ListByUploader(ctx context.Context, uploaderID string) ([]entity.AudioTrack, error)
	Delete(ctx context.Context, id string) error
}
