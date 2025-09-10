package cache

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type RateLimitInfo struct {
	Limit     int
	Remaining int
	Reset     time.Time
	mu        sync.RWMutex
}

type RateLimiter struct {
	limits map[string]*RateLimitInfo
	mu     sync.RWMutex
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limits: make(map[string]*RateLimitInfo),
	}
}

func (r *RateLimiter) UpdateFromHeaders(domain string, headers http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.limits[domain]; !exists {
		r.limits[domain] = &RateLimitInfo{}
	}

	info := r.limits[domain]
	info.mu.Lock()
	defer info.mu.Unlock()

	if limit := headers.Get("X-RateLimit-Limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			info.Limit = l
		}
	}

	if remaining := headers.Get("X-RateLimit-Remaining"); remaining != "" {
		if rem, err := strconv.Atoi(remaining); err == nil {
			info.Remaining = rem
		}
	}

	if reset := headers.Get("X-RateLimit-Reset"); reset != "" {
		if resetTime, err := strconv.ParseInt(reset, 10, 64); err == nil {
			info.Reset = time.Unix(resetTime, 0)
		}
	}
}

func (r *RateLimiter) ShouldWait(domain string) (bool, time.Duration) {
	r.mu.RLock()
	info, exists := r.limits[domain]
	r.mu.RUnlock()

	if !exists {
		return false, 0
	}

	info.mu.RLock()
	defer info.mu.RUnlock()

	if info.Remaining < 10 && time.Now().Before(info.Reset) {
		waitTime := time.Until(info.Reset)
		return true, waitTime
	}

	return false, 0
}

func (r *RateLimiter) Wait(ctx context.Context, domain string) error {
	shouldWait, duration := r.ShouldWait(domain)
	if !shouldWait {
		return nil
	}

	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *RateLimiter) GetInfo(domain string) (RateLimitInfo, bool) {
	r.mu.RLock()
	info, exists := r.limits[domain]
	r.mu.RUnlock()

	if !exists {
		return RateLimitInfo{}, false
	}

	info.mu.RLock()
	defer info.mu.RUnlock()

	return RateLimitInfo{
		Limit:     info.Limit,
		Remaining: info.Remaining,
		Reset:     info.Reset,
	}, true
}

type AdaptiveRateLimiter struct {
	base           *RateLimiter
	backoffTracker map[string]*BackoffInfo
	mu             sync.RWMutex
}

type BackoffInfo struct {
	Attempts      int
	LastAttempt   time.Time
	NextRetryTime time.Time
}

func NewAdaptiveRateLimiter() *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		base:           NewRateLimiter(),
		backoffTracker: make(map[string]*BackoffInfo),
	}
}

func (a *AdaptiveRateLimiter) RecordError(domain string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.backoffTracker[domain]; !exists {
		a.backoffTracker[domain] = &BackoffInfo{}
	}

	info := a.backoffTracker[domain]
	info.Attempts++
	info.LastAttempt = time.Now()

	backoffDuration := time.Duration(1<<uint(info.Attempts-1)) * time.Second
	if backoffDuration > 5*time.Minute {
		backoffDuration = 5 * time.Minute
	}

	info.NextRetryTime = time.Now().Add(backoffDuration)
}

func (a *AdaptiveRateLimiter) RecordSuccess(domain string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.backoffTracker, domain)
}

func (a *AdaptiveRateLimiter) CanProceed(domain string) (bool, time.Duration) {
	a.mu.RLock()
	info, exists := a.backoffTracker[domain]
	a.mu.RUnlock()

	if !exists {
		return true, 0
	}

	if time.Now().Before(info.NextRetryTime) {
		return false, time.Until(info.NextRetryTime)
	}

	return true, 0
}

func (a *AdaptiveRateLimiter) Wait(ctx context.Context, domain string) error {
	if err := a.base.Wait(ctx, domain); err != nil {
		return err
	}

	canProceed, waitTime := a.CanProceed(domain)
	if !canProceed {
		select {
		case <-time.After(waitTime):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (a *AdaptiveRateLimiter) UpdateFromHeaders(domain string, headers http.Header) {
	a.base.UpdateFromHeaders(domain, headers)
}

type RequestQueue struct {
	queue      chan func()
	maxWorkers int
	wg         sync.WaitGroup
}

func NewRequestQueue(maxWorkers int) *RequestQueue {
	rq := &RequestQueue{
		queue:      make(chan func(), 1000),
		maxWorkers: maxWorkers,
	}

	for i := 0; i < maxWorkers; i++ {
		go rq.worker()
	}

	return rq
}

func (rq *RequestQueue) worker() {
	for fn := range rq.queue {
		fn()
		rq.wg.Done()
	}
}

func (rq *RequestQueue) Submit(fn func()) {
	rq.wg.Add(1)
	rq.queue <- fn
}

func (rq *RequestQueue) Wait() {
	rq.wg.Wait()
}

func (rq *RequestQueue) Close() {
	close(rq.queue)
}

func ExponentialBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	duration := time.Duration(1<<uint(attempt-1)) * time.Second
	if duration > 30*time.Second {
		duration = 30 * time.Second
	}

	return duration
}

func RetryWithBackoff(ctx context.Context, maxAttempts int, fn func() error) error {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if attempt < maxAttempts {
			backoff := ExponentialBackoff(attempt)
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return fmt.Errorf("context cancelled: %w", ctx.Err())
			}
		}
	}

	return fmt.Errorf("max attempts reached: %w", lastErr)
}