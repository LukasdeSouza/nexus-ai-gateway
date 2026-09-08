// Package ratelimit provides Redis-backed sliding-window rate limiting.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	redisstore "github.com/LukasdeSouza/nexus-ai-gateway/internal/storage/redis"
)

// Scope defines the enforcement boundary.
type Scope string

const (
	ScopeOrg      Scope = "org"
	ScopeProject  Scope = "project"
	ScopeKey      Scope = "key"
	ScopeProvider Scope = "provider"
	ScopeModel    Scope = "model"
)

// Limit defines max requests over a given time duration.
type Limit struct {
	Requests int           `json:"requests"`
	Window   time.Duration `json:"window"`
}

// Result describes the outcome of a rate limit check.
type Result struct {
	Allowed   bool      `json:"allowed"`
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetAt   time.Time `json:"reset_at"`
}

// Limiter enforces rate limits using Redis.
type Limiter struct {
	cache *redisstore.Client
}

// NewLimiter creates a new Limiter instance.
func NewLimiter(cache *redisstore.Client) *Limiter {
	return &Limiter{cache: cache}
}

// Check evaluates whether the request is permitted within the rate limit window.
func (l *Limiter) Check(ctx context.Context, scope Scope, id string, limit Limit) (*Result, error) {
	if limit.Requests <= 0 || limit.Window <= 0 {
		return &Result{Allowed: true, Remaining: 999999, Limit: limit.Requests, ResetAt: time.Now().Add(time.Minute)}, nil
	}

	// Fail open if no redis client is configured (e.g. testing or cache down)
	if l.cache == nil {
		return &Result{Allowed: true, Remaining: limit.Requests, Limit: limit.Requests, ResetAt: time.Now().Add(limit.Window)}, nil
	}

	now := time.Now().UTC()
	windowSeconds := int64(limit.Window.Seconds())
	if windowSeconds <= 0 {
		windowSeconds = 60
	}

	currentBucket := now.Unix() / windowSeconds
	key := fmt.Sprintf("ratelimit:%s:%s:%d", scope, id, currentBucket)

	count, err := l.cache.IncrementWithExpiry(ctx, key, limit.Window*2)
	if err != nil {
		// On Redis error, fail open so we don't break user traffic
		return &Result{Allowed: true, Remaining: limit.Requests, Limit: limit.Requests, ResetAt: now.Add(limit.Window)}, nil
	}

	resetAt := time.Unix((currentBucket+1)*windowSeconds, 0)
	remaining := limit.Requests - int(count)
	if remaining < 0 {
		remaining = 0
	}

	allowed := count <= int64(limit.Requests)
	return &Result{
		Allowed:   allowed,
		Remaining: remaining,
		Limit:     limit.Requests,
		ResetAt:   resetAt,
	}, nil
}
