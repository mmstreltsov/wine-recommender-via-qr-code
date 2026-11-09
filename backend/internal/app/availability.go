package app

import (
	"context"
	"log"
	"sync"
	"time"

	"svoyshef/internal/store"
)

const availabilityRefreshTimeout = 2 * time.Second

// AvailabilityCache — TTL-кэш недоступных позиций (FR-36/37), схема stale-while-revalidate:
// мьютекс не удерживается на время БД-запроса; всем вызывающим сразу отдаётся текущий снапшот,
// обновление идёт фоновой горутиной. Снапшот иммутабелен после установки и заменяется целиком;
// ошибка обновления не меняет ответы (остаётся последний снапшот).
type AvailabilityCache struct {
	src         store.AvailabilityStore
	ttl         time.Duration
	mu          sync.Mutex
	unavailable map[int64]struct{}
	refreshedAt time.Time
	refreshing  bool
}

func NewAvailabilityCache(src store.AvailabilityStore, ttl time.Duration) *AvailabilityCache {
	return &AvailabilityCache{
		src:         src,
		ttl:         ttl,
		unavailable: make(map[int64]struct{}),
	}
}

func (c *AvailabilityCache) IsAvailable(ctx context.Context, itemID int64) bool {
	_, disabled := c.UnavailableIDs(ctx)[itemID]
	return !disabled
}

// UnavailableIDs возвращает текущий иммутабельный снапшот (только чтение).
func (c *AvailabilityCache) UnavailableIDs(ctx context.Context) map[int64]struct{} {
	c.mu.Lock()
	if time.Since(c.refreshedAt) >= c.ttl && !c.refreshing {
		c.refreshing = true
		c.refreshedAt = time.Now()
		go c.refresh()
	}
	snapshot := c.unavailable
	c.mu.Unlock()
	return snapshot
}

func (c *AvailabilityCache) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), availabilityRefreshTimeout)
	defer cancel()
	ids, err := c.src.Unavailable(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshing = false
	if err != nil {
		log.Printf("ERROR operation=availability_refresh code=internal keeping_last_snapshot items=%d", len(c.unavailable))
		return
	}
	fresh := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		fresh[id] = struct{}{}
	}
	c.unavailable = fresh
}
