package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// LoginLimiter throttles login attempts per key (client IP + email).
// It is in-memory, which is fine for a single app instance.
type LoginLimiter struct {
	mu       sync.Mutex
	limiters map[string]*entry
	every    time.Duration
	burst    int
}

type entry struct {
	lim  *rate.Limiter
	seen time.Time
}

// NewLoginLimiter allows `burst` attempts, then one more every `every`.
func NewLoginLimiter(every time.Duration, burst int) *LoginLimiter {
	l := &LoginLimiter{limiters: map[string]*entry{}, every: every, burst: burst}
	go l.gc()
	return l
}

// Allow reports whether another attempt for key may proceed.
func (l *LoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.limiters[key]
	if !ok {
		e = &entry{lim: rate.NewLimiter(rate.Every(l.every), l.burst)}
		l.limiters[key] = e
	}
	e.seen = time.Now()
	return e.lim.Allow()
}

func (l *LoginLimiter) gc() {
	for range time.Tick(10 * time.Minute) {
		l.mu.Lock()
		for k, e := range l.limiters {
			if time.Since(e.seen) > time.Hour {
				delete(l.limiters, k)
			}
		}
		l.mu.Unlock()
	}
}
