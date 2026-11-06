// Modifications copyright (c) 2026 Bojan van der Heide

package oauth

import (
	"sync"
	"time"
)

// limiter is a fixed-window counter per key. It exists to make guessing the
// admin password and mass-registering clients unattractive, not to be a
// distributed rate limiter: one process serves this endpoint, and a restart
// resetting the counters is acceptable.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	count int
	reset time.Time
}

func newLimiter() *limiter {
	return &limiter{buckets: make(map[string]*bucket)}
}

// allow consumes one slot for key. It returns false when the window is full.
func (l *limiter) allow(key string, limit int, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) > 10000 {
		for k, b := range l.buckets {
			if b.reset.Before(now) {
				delete(l.buckets, k)
			}
		}
	}

	b, ok := l.buckets[key]
	if !ok || b.reset.Before(now) {
		l.buckets[key] = &bucket{count: 1, reset: now.Add(window)}
		return true
	}
	if b.count >= limit {
		return false
	}
	b.count++
	return true
}

// reset clears the counter for key, used after a successful login so a person
// who mistyped twice is not penalised for the rest of the window.
func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.buckets, key)
	l.mu.Unlock()
}
