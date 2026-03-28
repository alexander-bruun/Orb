// Package activity provides a simple emitter for recording user activity events.
package activity

import (
	"context"
	"log/slog"

	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/google/uuid"
)

// Emitter records user activity events into the store.
type Emitter struct {
	db *store.Store
}

// New returns a new Emitter.
func New(db *store.Store) *Emitter {
	return &Emitter{db: db}
}

// Record inserts an activity row. Errors are logged but not returned so that
// activity recording never blocks the primary operation.
func (e *Emitter) Record(ctx context.Context, userID, actType, entityType, entityID string, meta map[string]any) {
	err := e.db.InsertActivity(ctx, store.InsertActivityParams{
		ID:         uuid.New().String(),
		UserID:     userID,
		Type:       actType,
		EntityType: entityType,
		EntityID:   entityID,
		Metadata:   meta,
	})
	if err != nil {
		slog.Warn("activity: failed to record event", "type", actType, "err", err)
	}
}
