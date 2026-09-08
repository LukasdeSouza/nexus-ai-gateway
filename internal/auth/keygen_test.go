package auth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/auth"
)

func TestGenerateKey_Environments(t *testing.T) {
	tests := []struct {
		env    string
		prefix string
	}{
		{"live", "ngk_live_"},
		{"production", "ngk_live_"},
		{"test", "ngk_test_"},
		{"development", "ngk_test_"},
		{"default", "ngk_"},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			key, pfx, err := auth.GenerateKey(tt.env)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(key, tt.prefix))
			assert.NotEmpty(t, pfx)
			assert.True(t, auth.IsValidKeyFormat(key))
		})
	}
}

func TestGenerateKey_Uniqueness(t *testing.T) {
	key1, _, err := auth.GenerateKey("live")
	require.NoError(t, err)

	key2, _, err := auth.GenerateKey("live")
	require.NoError(t, err)

	assert.NotEqual(t, key1, key2)
}

func TestExtractPrefix(t *testing.T) {
	key := "ngk_live_1234567890abcdef"
	pfx := auth.ExtractPrefix(key)
	assert.Equal(t, "ngk_live_123...", pfx)
}
