package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLimiter_AllowWithinRate(t *testing.T) {
	lim := NewLimiter(5, time.Second)
	for i := 0; i < 5; i++ {
		if !lim.Allow("key") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestLimiter_DenyOverRate(t *testing.T) {
	lim := NewLimiter(3, time.Second)
	for i := 0; i < 3; i++ {
		if !lim.Allow("key") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if lim.Allow("key") {
		t.Errorf("request over rate limit should be denied")
	}
}

func TestLimiter_DifferentKeys(t *testing.T) {
	lim := NewLimiter(2, time.Second)
	for i := 0; i < 2; i++ {
		if !lim.Allow("a") {
			t.Fatalf("request %d for key 'a' should be allowed", i+1)
		}
		if !lim.Allow("b") {
			t.Fatalf("request %d for key 'b' should be allowed", i+1)
		}
	}
	if lim.Allow("a") {
		t.Errorf("key 'a' should be rate limited")
	}
	if lim.Allow("b") {
		t.Errorf("key 'b' should be rate limited")
	}
}

func TestLimiter_WindowReset(t *testing.T) {
	lim := NewLimiter(2, 50*time.Millisecond)
	for i := 0; i < 2; i++ {
		if !lim.Allow("key") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if lim.Allow("key") {
		t.Errorf("request over rate should be denied")
	}

	time.Sleep(110 * time.Millisecond)

	if !lim.Allow("key") {
		t.Errorf("request after window reset should be allowed")
	}
}

func TestLimiter_Concurrent(t *testing.T) {
	const rate = 50
	lim := NewLimiter(rate, time.Second)
	var wg sync.WaitGroup
	counts := make([]int64, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("user-%d", id)
			for j := 0; j < 100; j++ {
				if lim.Allow(key) {
					counts[id]++
				}
			}
		}(i)
	}
	wg.Wait()

	for i, c := range counts {
		if c > rate {
			t.Errorf("key user-%d: allowed %d requests, expected at most %d", i, c, rate)
		}
	}
}
