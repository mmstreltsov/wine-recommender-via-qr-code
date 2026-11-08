package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"svoyshef/internal/domain"
)

type EventStorePG struct {
	pool *pgxpool.Pool
}

func NewEventStore(pool *pgxpool.Pool) *EventStorePG {
	return &EventStorePG{pool: pool}
}

func marshalPayload(payload map[string]any) ([]byte, error) {
	if len(payload) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(payload)
}

func (s *EventStorePG) Insert(ctx context.Context, e domain.Event) error {
	payload, err := marshalPayload(e.Payload)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO events (session_id, table_id, name, payload)
		VALUES ($1::uuid, $2, $3, $4)`, e.SessionID, e.TableID, e.Name, payload)
	return err
}

func (s *EventStorePG) InsertBatch(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range events {
		payload, err := marshalPayload(e.Payload)
		if err != nil {
			return err
		}
		batch.Queue(`
			INSERT INTO events (session_id, table_id, name, payload)
			VALUES ($1::uuid, $2, $3, $4)`, e.SessionID, e.TableID, e.Name, payload)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	for range events {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}
