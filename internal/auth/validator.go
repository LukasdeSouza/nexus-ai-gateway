package auth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	redisstore "github.com/LukasdeSouza/nexus-ai-gateway/internal/storage/redis"
)

// KeyStore abstracts database lookup for API keys.
type KeyStore interface {
	GetByHash(ctx context.Context, hash string) (*domain.APIKey, error)
}

// Validator validates incoming plaintext API keys with Redis caching.
type Validator struct {
	keys     KeyStore
	cache    *redisstore.Client
	cacheTTL time.Duration
}

// NewValidator creates a key Validator with cache.
func NewValidator(keys KeyStore, cache *redisstore.Client, cacheTTL time.Duration) *Validator {
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}
	return &Validator{
		keys:     keys,
		cache:    cache,
		cacheTTL: cacheTTL,
	}
}

// Validate checks the key against cache and database, ensuring it is active and not expired.
func (v *Validator) Validate(ctx context.Context, plaintext string) (*domain.APIKey, error) {
	if plaintext == "" {
		return nil, domain.ErrUnauthorized
	}

	hash, err := HashKey(plaintext)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	cacheKey := "apikey:" + hash

	// 1. Try Redis cache if available
	if v.cache != nil {
		if cachedJSON, err := v.cache.Get(ctx, cacheKey); err == nil && cachedJSON != "" {
			var key domain.APIKey
			if jsonErr := json.Unmarshal([]byte(cachedJSON), &key); jsonErr == nil {
				return validateKeyStatus(&key)
			}
		}
	}

	// 2. Query persistent store
	key, err := v.keys.GetByHash(ctx, hash)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	if key == nil {
		return nil, domain.ErrUnauthorized
	}

	// 3. Cache valid key in Redis
	if v.cache != nil {
		if data, err := json.Marshal(key); err == nil {
			_ = v.cache.SetWithTTL(ctx, cacheKey, string(data), v.cacheTTL)
		}
	}

	return validateKeyStatus(key)
}

func validateKeyStatus(key *domain.APIKey) (*domain.APIKey, error) {
	if key.Status == domain.KeyStatusRevoked {
		return nil, domain.ErrKeyRevoked
	}
	if key.IsExpired() {
		return nil, domain.ErrKeyExpired
	}
	if !key.IsActive() {
		return nil, domain.ErrUnauthorized
	}
	return key, nil
}
