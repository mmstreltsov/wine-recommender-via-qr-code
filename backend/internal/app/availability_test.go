package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"svoyshef/internal/store"
)

type fakeAvailabilityStore struct {
	mu    sync.Mutex
	ids   []int64
	err   error
	calls int
}

func (f *fakeAvailabilityStore) setIDs(ids []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = ids
}

func (f *fakeAvailabilityStore) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeAvailabilityStore) callsCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeAvailabilityStore) Unavailable(ctx context.Context) ([]int64, error) {
	f.mu.Lock()
	f.calls++
	ids, err := f.ids, f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (f *fakeAvailabilityStore) Upsert(ctx context.Context, itemID int64, isAvailable bool) error {
	return nil
}

func (f *fakeAvailabilityStore) Records(ctx context.Context) ([]store.AvailabilityRecord, error) {
	return nil, nil
}

func waitForCond(t *testing.T, timeout time.Duration, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting: %s", what)
}

func TestFR36_TTLCacheRefreshesAfterTTL(t *testing.T) {
	src := &fakeAvailabilityStore{ids: []int64{1}}
	cache := NewAvailabilityCache(src, 30*time.Millisecond)
	ctx := context.Background()
	cache.IsAvailable(ctx, 1)
	waitForCond(t, time.Second, "first async refresh applies snapshot", func() bool {
		return !cache.IsAvailable(ctx, 1)
	})
	if got := src.callsCount(); got < 1 {
		t.Fatalf("expected at least 1 store call, got %d", got)
	}
	callsAfterFirst := src.callsCount()
	cache.IsAvailable(ctx, 2)
	if got := src.callsCount(); got != callsAfterFirst {
		t.Fatalf("within TTL no refresh expected, calls=%d", got)
	}
	src.setIDs([]int64{2})
	time.Sleep(40 * time.Millisecond)
	waitForCond(t, time.Second, "second refresh after TTL applies new snapshot", func() bool {
		return !cache.IsAvailable(ctx, 2) && cache.IsAvailable(ctx, 1)
	})
}

func TestFR36_ErrorKeepsLastSnapshot(t *testing.T) {
	src := &fakeAvailabilityStore{ids: []int64{5}}
	cache := NewAvailabilityCache(src, 20*time.Millisecond)
	ctx := context.Background()
	cache.IsAvailable(ctx, 5)
	waitForCond(t, time.Second, "first refresh applies snapshot", func() bool {
		return !cache.IsAvailable(ctx, 5)
	})
	src.setErr(errors.New("db down"))
	time.Sleep(25 * time.Millisecond)
	cache.IsAvailable(ctx, 5)
	waitForCond(t, time.Second, "failed refresh attempt happened", func() bool {
		return src.callsCount() >= 2
	})
	if cache.IsAvailable(ctx, 5) {
		t.Fatal("on refresh error the last snapshot must be kept")
	}
	snapshot := cache.UnavailableIDs(ctx)
	if _, ok := snapshot[5]; !ok {
		t.Fatal("snapshot must contain item 5")
	}
}

func TestFR36_DefaultAvailableWhenEmptyTable(t *testing.T) {
	src := &fakeAvailabilityStore{ids: nil}
	cache := NewAvailabilityCache(src, time.Minute)
	if !cache.IsAvailable(context.Background(), 42) {
		t.Fatal("empty table means everything available")
	}
}

// SWR: фоновый refresh не блокирует вызывающих — БД-запрос не под мьютексом.
func TestFR36_RefreshDoesNotBlockCallers(t *testing.T) {
	src := &slowAvailabilityStore{delay: 150 * time.Millisecond, ids: []int64{1}}
	cache := NewAvailabilityCache(src, 20*time.Millisecond)
	ctx := context.Background()
	cache.IsAvailable(ctx, 1)
	waitForCond(t, time.Second, "first refresh applies snapshot", func() bool {
		return !cache.IsAvailable(ctx, 1)
	})
	src.setIDs([]int64{2})
	time.Sleep(25 * time.Millisecond)
	start := time.Now()
	snapshot := cache.UnavailableIDs(ctx)
	elapsed := time.Since(start)
	if _, off := snapshot[1]; !off {
		t.Fatal("stale snapshot must be served immediately")
	}
	if elapsed >= 75*time.Millisecond {
		t.Fatalf("UnavailableIDs must not wait for refresh, took %s", elapsed)
	}
	waitForCond(t, time.Second, "background refresh completes", func() bool {
		return !cache.IsAvailable(ctx, 2) && cache.IsAvailable(ctx, 1)
	})
}

type slowAvailabilityStore struct {
	mu    sync.Mutex
	delay time.Duration
	ids   []int64
	calls int
}

func (f *slowAvailabilityStore) setIDs(ids []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = ids
}

func (f *slowAvailabilityStore) Unavailable(ctx context.Context) ([]int64, error) {
	f.mu.Lock()
	f.calls++
	ids, delay := f.ids, f.delay
	f.mu.Unlock()
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return ids, nil
}

func (f *slowAvailabilityStore) Upsert(ctx context.Context, itemID int64, isAvailable bool) error {
	return nil
}

func (f *slowAvailabilityStore) Records(ctx context.Context) ([]store.AvailabilityRecord, error) {
	return nil, nil
}

var _ store.AvailabilityStore = (*slowAvailabilityStore)(nil)
