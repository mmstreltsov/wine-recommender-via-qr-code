package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SessionStorePG struct {
	pool *pgxpool.Pool
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStorePG {
	return &SessionStorePG{pool: pool}
}

func (s *SessionStorePG) Create(ctx context.Context, id string, tableID *int) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions (id, table_id) VALUES ($1::uuid, $2)`, id, tableID)
	return err
}

func (s *SessionStorePG) Get(ctx context.Context, id string) (bool, *int, error) {
	var tableID *int
	err := s.pool.QueryRow(ctx, `SELECT table_id FROM sessions WHERE id = $1::uuid`, id).Scan(&tableID)
	if err == pgx.ErrNoRows {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return true, tableID, nil
}
