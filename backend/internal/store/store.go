package store

import (
	"context"
	"errors"
	"time"

	"svoyshef/internal/domain"
)

var ErrQtyLimit = errors.New("qty limit exceeded")

type SessionStore interface {
	Create(ctx context.Context, id string, tableID *int, credentialHash [32]byte, maxAge time.Duration) error
	GetByCredentialHash(ctx context.Context, credentialHash [32]byte) (id string, tableID *int, err error)
}

type CartStore interface {
	Lines(ctx context.Context, sessionID string) ([]domain.CartLine, error)
	AddItem(ctx context.Context, sessionID string, itemID int64, addedFrom string) (qty int, err error)
	SetQty(ctx context.Context, sessionID string, itemID int64, qty int) (found bool, prevQty int, err error)
	RemoveItem(ctx context.Context, sessionID string, itemID int64) (found bool, err error)
}

type EventStore interface {
	Insert(ctx context.Context, e domain.Event) error
	InsertBatch(ctx context.Context, events []domain.Event) error
}

type AvailabilityRecord struct {
	ItemID      int64
	IsAvailable bool
	UpdatedAt   time.Time
}

type AvailabilityStore interface {
	Unavailable(ctx context.Context) ([]int64, error)
	Upsert(ctx context.Context, itemID int64, isAvailable bool) error
	Records(ctx context.Context) ([]AvailabilityRecord, error)
}
