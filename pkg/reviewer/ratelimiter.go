package reviewer

import (
	"context"
	"sync"
	"time"
)

type RateLimiter struct {
	requestsPerMinute int
	burst             int
	tokens            int
	lastRefill        time.Time
	mu                sync.Mutex
	waitTime          time.Duration
}

func NewRateLimiter(requestsPerMinute, burst int, waitTime time.Duration) *RateLimiter {
	if burst <= 0 {
		burst = requestsPerMinute / 10
		if burst < 1 {
			burst = 1
		}
	}
	return &RateLimiter{
		requestsPerMinute: requestsPerMinute,
		burst:             burst,
		tokens:            burst,
		lastRefill:        time.Now(),
		waitTime:          waitTime,
	}
}

// Wait blocks until a token is availabele or context is cancelled
func (r *RateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()

	r.refillTokens()
	if r.tokens > 0 {
		r.tokens--
		r.mu.Unlock()
		return nil
	}

	//Calculate wait time for the next token
	tokensPerSecond := float64(r.requestsPerMinute) / 60.0
	waitDuration := time.Duration(float64(time.Second) / tokensPerSecond)

	r.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(waitDuration):
		return nil
	}

}

// refillTokens refills tokens based on elapsed time
func (r *RateLimiter) refillTokens() {
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)

	// Calculate tokens to add
	tokensPerSecond := float64(r.requestsPerMinute) / 60.0
	tokensToAdd := int(elapsed.Seconds() * tokensPerSecond)
	if tokensToAdd > 0 {
		r.tokens += tokensToAdd
		if r.tokens > r.burst {
			r.tokens = r.burst
		}
		r.lastRefill = now
	}
}

// Try returns immediately if tokens is available
func (r *RateLimiter) Try() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refillTokens()
	if r.tokens > 0 {
		r.tokens--
		return true
	}
	return false
}

// SetRate changes the rate limit dynamically
func (r *RateLimiter) SetRate(requestsPerMinute, burst int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requestsPerMinute = requestsPerMinute
	if burst > 0 {
		r.burst = burst
	}
	// Recalculate tokens based on new rate
	r.refillTokens()
}

// Stats returns current rate limiter statistics
func (r *RateLimiter) Stats() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refillTokens()

	return map[string]interface{}{
		"requests_per_minute": r.requestsPerMinute,
		"burst":               r.burst,
		"available_tokens":    r.tokens,
		"last_refill":         r.lastRefill,
		"max_wait_time":       r.waitTime,
	}
}
