package wrap

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

const maxTrackedClients = 10000

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// RateLimiter implements a memory-bounded token bucket limiter keyed by client IP.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64
	capacity float64
	window   time.Duration
}

// NewRateLimiter creates a limiter allowing maxRequests per window with burst capacity.
func NewRateLimiter(maxRequests int, window time.Duration, burst int) *RateLimiter {
	if burst < 1 {
		burst = maxRequests
	}
	return &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     float64(maxRequests) / window.Seconds(),
		capacity: float64(burst),
		window:   window,
	}
}

// Allow checks if the client key has an available token and writes standard rate limit headers.
func (rl *RateLimiter) Allow(w http.ResponseWriter, key string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if len(rl.buckets) >= maxTrackedClients {
		rl.evictStale(now)
	}

	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{
			tokens:   rl.capacity - 1,
			lastSeen: now,
		}
		rl.buckets[key] = b
		rl.writeHeaders(w, int(b.tokens), 0)
		return true
	}

	elapsed := now.Sub(b.lastSeen).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rl.rate
		if b.tokens > rl.capacity {
			b.tokens = rl.capacity
		}
		b.lastSeen = now
	}

	if b.tokens < 1.0 {
		deficit := 1.0 - b.tokens
		retrySecs := int(deficit/rl.rate) + 1
		if retrySecs < 1 {
			retrySecs = 1
		}
		rl.writeHeaders(w, 0, retrySecs)
		return false
	}

	b.tokens -= 1.0
	rl.writeHeaders(w, int(b.tokens), 0)
	return true
}

func (rl *RateLimiter) writeHeaders(w http.ResponseWriter, remaining int, retryAfter int) {
	if w == nil {
		return
	}
	h := w.Header()
	h.Set("X-RateLimit-Limit", fmt.Sprintf("%d", int(rl.capacity)))
	h.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	if retryAfter > 0 {
		h.Set("Retry-After", fmt.Sprintf("%d", retryAfter))
	}
}

func (rl *RateLimiter) evictStale(now time.Time) {
	cutoff := now.Add(-rl.window * 2)
	var oldestKey string
	var oldestTime time.Time

	for k, b := range rl.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(rl.buckets, k)
			continue
		}
		if oldestKey == "" || b.lastSeen.Before(oldestTime) {
			oldestKey = k
			oldestTime = b.lastSeen
		}
	}

	if len(rl.buckets) >= maxTrackedClients && oldestKey != "" {
		delete(rl.buckets, oldestKey)
	}
}
