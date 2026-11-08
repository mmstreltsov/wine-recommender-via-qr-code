package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AvailabilityStorePG struct {
	pool *pgxpool.Pool
}

func NewAvailabilityStore(pool *pgxpool.Pool) *AvailabilityStorePG {
	return &AvailabilityStorePG{pool: pool}
}

func (s *AvailabilityStorePG) Unavailable(ctx context.Context) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT item_id FROM item_availability
		WHERE is_available = false
		ORDER BY item_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *AvailabilityStorePG) Upsert(ctx context.Context, itemID int64, isAvailable bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO item_availability (item_id, is_available)
		VALUES ($1, $2)
		ON CONFLICT (item_id) DO UPDATE SET is_available = $2, updated_at = now()`, itemID, isAvailable)
	return err
}

func (s *AvailabilityStorePG) Records(ctx context.Context) ([]AvailabilityRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT item_id, is_available, updated_at
		FROM item_availability
		ORDER BY item_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]AvailabilityRecord, 0)
	for rows.Next() {
		var r AvailabilityRecord
		if err := rows.Scan(&r.ItemID, &r.IsAvailable, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
