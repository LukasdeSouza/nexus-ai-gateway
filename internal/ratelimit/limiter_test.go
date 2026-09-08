package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/ratelimit"
)

func TestLimiter_FailOpenWithoutRedis(t *testing.T) {
	limiter := ratelimit.NewLimiter(nil)

	res, err := limiter.Check(context.Background(), ratelimit.ScopeProject, "prj_test", ratelimit.Limit{
		Requests: 100,
		Window:   time.Minute,
	})

	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 100, res.Remaining)
}
