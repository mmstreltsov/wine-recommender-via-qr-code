package store

import (
	"context"
	"encoding/json"
	"time"

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

// ExportedEvent — строка выгрузки AN-04; Payload — канонический текст jsonb
// как хранится в БД (payload::text), без повторной сериализации в Go.
type ExportedEvent struct {
	TS        time.Time
	SessionID string
	TableID   *int
	Name      string
	Payload   string
}

// ExportRows выбирает события для export-events (events.md §4): from/toExcl —
// полуинтервал [from, toExcl); nil = граница открыта; порядок ts, id.
func (s *EventStorePG) ExportRows(ctx context.Context, from, toExcl *time.Time) ([]ExportedEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ts, session_id, table_id, name, payload::text
		FROM events
		WHERE ($1::timestamptz IS NULL OR ts >= $1)
			AND ($2::timestamptz IS NULL OR ts < $2)
		ORDER BY ts, id`, from, toExcl)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExportedEvent
	for rows.Next() {
		var e ExportedEvent
		if err := rows.Scan(&e.TS, &e.SessionID, &e.TableID, &e.Name, &e.Payload); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
