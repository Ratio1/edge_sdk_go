package httpx

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// Backoff implements exponential backoff with optional jitter.
type Backoff struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
	Jitter    float64

	mu   sync.Mutex
	rand *rand.Rand
}

// NewBackoff returns a Backoff initialized with the supplied parameters.
func NewBackoff(base, max time.Duration, jitter float64) Backoff {
	if base <= 0 {
		base = 50 * time.Millisecond
	}
	if max <= 0 {
		max = time.Second
	}
	if jitter < 0 {
		jitter = 0
	}
	return Backoff{
		BaseDelay: base,
		MaxDelay:  max,
		Jitter:    jitter,
		rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ForAttempt returns the backoff duration for the given attempt (0-indexed).
func (b *Backoff) ForAttempt(attempt int) time.Duration {
	if attempt <= 0 {
		return b.addJitter(b.BaseDelay)
	}

	exp := float64(uint(1) << uint(attempt))
	delay := time.Duration(float64(b.BaseDelay) * exp)
	if delay <= 0 || delay > b.MaxDelay {
		delay = b.MaxDelay
	}
	return b.addJitter(delay)
}

func (b *Backoff) addJitter(delay time.Duration) time.Duration {
	if b.Jitter == 0 || delay <= 0 {
		return delay
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	factor := 1 + (b.rand.Float64()*2-1)*math.Min(b.Jitter, 1)
	if factor < 0 {
		factor = 0
	}
	return time.Duration(float64(delay) * factor)
}
