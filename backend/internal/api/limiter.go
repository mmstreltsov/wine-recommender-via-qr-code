package api

import (
	"sync"
	"time"
)

type slidingWindowLimiter struct {
	mu     sync.Mutex
	events map[string][]time.Time
	max    int
	window time.Duration
	calls  uint64
}

func newLimiter(max int, window time.Duration) *slidingWindowLimiter {
	return &slidingWindowLimiter{events: make(map[string][]time.Time), max: max, window: window}
}

func (l *slidingWindowLimiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	times := l.events[key]
	kept := times[:0]
	for _, t := range times {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.events[key] = kept
		l.sweepLocked(now)
		return false
	}
	l.events[key] = append(kept, now)
	l.sweepLocked(now)
	return true
}

func (l *slidingWindowLimiter) sweepLocked(now time.Time) {
	l.calls++
	if l.calls%1024 != 0 {
		return
	}
	for k, times := range l.events {
		stale := true
		for _, t := range times {
			if now.Sub(t) < l.window {
				stale = false
				break
			}
		}
		if stale {
			delete(l.events, k)
		}
	}
}
