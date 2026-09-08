package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/auth"
)

func TestHashKey_Consistency(t *testing.T) {
	key := "ngk_live_testkey123456789"

	hash1, err := auth.HashKey(key)
	require.NoError(t, err)

	hash2, err := auth.HashKey(key)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2)
}

func TestHashKey_DifferentKeys(t *testing.T) {
	hash1, err := auth.HashKey("ngk_live_key1")
	require.NoError(t, err)

	hash2, err := auth.HashKey("ngk_live_key2")
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2)
}

func TestCompareKey(t *testing.T) {
	key := "ngk_live_securetoken"
	hash := auth.MustHashKey(key)

	assert.True(t, auth.CompareKey(key, hash))
	assert.False(t, auth.CompareKey("wrong_key", hash))
}
