package ratelimit

import (
	"sync"
	"time"
)

const numShards = 256

// ShardedSlidingWindowLimiter distributes keys across shards to avoid global lock contention.
type ShardedSlidingWindowLimiter struct {
	shards [numShards]shard
	rate   int
	window time.Duration
}

type shard struct {
	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	count     int
	prevCount int
	currStart time.Time
}

// NewLimiter creates a new sharded sliding window rate limiter.
func NewLimiter(rate int, w time.Duration) *ShardedSlidingWindowLimiter {
	l := &ShardedSlidingWindowLimiter{
		rate:   rate,
		window: w,
	}
	for i := range l.shards {
		l.shards[i].windows = make(map[string]*window)
	}
	return l
}

// Allow reports whether a request for the given key is permitted.
func (l *ShardedSlidingWindowLimiter) Allow(key string) bool {
	s := l.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	w, ok := s.windows[key]
	if !ok {
		s.windows[key] = &window{count: 1, currStart: now}
		return true
	}

	elapsed := now.Sub(w.currStart)
	if elapsed >= l.window {
		if elapsed >= 2*l.window {
			// More than two windows elapsed; previous window data is stale.
			w.prevCount = 0
		} else {
			w.prevCount = w.count
		}
		w.count = 0
		w.currStart = now
		elapsed = 0
	}

	weight := 1.0 - float64(elapsed)/float64(l.window)
	estimate := float64(w.prevCount)*weight + float64(w.count)

	if estimate >= float64(l.rate) {
		return false
	}

	w.count++
	return true
}

func (l *ShardedSlidingWindowLimiter) getShard(key string) *shard {
	return &l.shards[fnv32a(key)%numShards]
}

func fnv32a(s string) uint32 {
	const (
		offset32 = uint32(2166136261)
		prime32  = uint32(16777619)
	)
	h := offset32
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return h
}
