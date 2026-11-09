package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SessionStorePG struct {
	pool *pgxpool.Pool
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStorePG {
	return &SessionStorePG{pool: pool}
}

func (s *SessionStorePG) Create(ctx context.Context, id string, tableID *int, credentialHash [32]byte, maxAge time.Duration) error {
	if maxAge < time.Second {
		return errors.New("invalid session lifetime")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, table_id, credential_hash, expires_at)
		VALUES ($1::uuid, $2, $3, now() + $4::bigint * interval '1 second')`,
		id, tableID, credentialHash[:], int64(maxAge/time.Second))
	return err
}

func (s *SessionStorePG) GetByCredentialHash(ctx context.Context, credentialHash [32]byte) (string, *int, error) {
	var id string
	var tableID *int
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, table_id FROM sessions
		WHERE credential_hash = $1 AND expires_at > now()`, credentialHash[:]).Scan(&id, &tableID)
	if err == pgx.ErrNoRows {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return id, tableID, nil
}
