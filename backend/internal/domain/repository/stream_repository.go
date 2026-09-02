package repository

import (
	"context"

	"github.com/streampulse/backend/internal/domain/entity"
)

// StreamRepository persists stream metadata. The live audio itself never
// touches this interface — it is fanned out in memory by the streaming hub.
type StreamRepository interface {
	Create(ctx context.Context, stream *entity.Stream) error
	FindByID(ctx context.Context, id string) (*entity.Stream, error)
	List(ctx context.Context) ([]entity.Stream, error)
	ListByStatus(ctx context.Context, status entity.StreamStatus) ([]entity.Stream, error)
	UpdateStatus(ctx context.Context, id string, status entity.StreamStatus) error
	Delete(ctx context.Context, id string) error
}
