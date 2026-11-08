package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"svoyshef/internal/domain"
)

type CartStorePG struct {
	pool *pgxpool.Pool
}

func NewCartStore(pool *pgxpool.Pool) *CartStorePG {
	return &CartStorePG{pool: pool}
}

func (s *CartStorePG) Lines(ctx context.Context, sessionID string) ([]domain.CartLine, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT item_id, qty, added_from
		FROM cart_items
		WHERE session_id = $1::uuid
		ORDER BY added_at ASC, item_id ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := make([]domain.CartLine, 0)
	for rows.Next() {
		var l domain.CartLine
		if err := rows.Scan(&l.ItemID, &l.Qty, &l.AddedFrom); err != nil {
			return nil, err
		}
		l.Available = true
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (s *CartStorePG) AddItem(ctx context.Context, sessionID string, itemID int64, addedFrom string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO carts (session_id) VALUES ($1::uuid)
		ON CONFLICT (session_id) DO UPDATE SET updated_at = now()`, sessionID); err != nil {
		return 0, err
	}
	var qty int
	err = tx.QueryRow(ctx, `
		INSERT INTO cart_items (session_id, item_id, qty, added_from)
		VALUES ($1::uuid, $2, 1, $3)
		ON CONFLICT (session_id, item_id) DO UPDATE
		SET qty = cart_items.qty + 1
		WHERE cart_items.qty < 20
		RETURNING qty`, sessionID, itemID, addedFrom).Scan(&qty)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrQtyLimit
	}
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return qty, nil
}

func (s *CartStorePG) SetQty(ctx context.Context, sessionID string, itemID int64, qty int) (bool, int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var prev int
	err = tx.QueryRow(ctx, `
		SELECT qty FROM cart_items
		WHERE session_id = $1::uuid AND item_id = $2
		FOR UPDATE`, sessionID, itemID).Scan(&prev)
	if err == pgx.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cart_items SET qty = $3
		WHERE session_id = $1::uuid AND item_id = $2`, sessionID, itemID, qty); err != nil {
		return false, 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE carts SET updated_at = now() WHERE session_id = $1::uuid`, sessionID); err != nil {
		return false, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return true, prev, nil
}

func (s *CartStorePG) RemoveItem(ctx context.Context, sessionID string, itemID int64) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		DELETE FROM cart_items
		WHERE session_id = $1::uuid AND item_id = $2`, sessionID, itemID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE carts SET updated_at = now() WHERE session_id = $1::uuid`, sessionID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
