package gemini_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider/gemini"
)

func TestGeminiAdapter_Chat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models/gemini-1.5-flash:generateContent", r.URL.Path)
		assert.Equal(t, "test-gemini-key", r.URL.Query().Get("key"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [{"text": "Hello from Gemini!"}],
					"role": "model"
				},
				"finishReason": "STOP",
				"index": 0
			}],
			"usageMetadata": {
				"promptTokenCount": 11,
				"candidatesTokenCount": 14,
				"totalTokenCount": 25
			}
		}`))
	}))
	defer server.Close()

	adapter := gemini.NewAdapter(server.URL, "test-gemini-key")
	resp, err := adapter.Chat(context.Background(), &provider.ChatRequest{
		Model: "gemini-1.5-flash",
		Messages: []provider.Message{
			{Role: "user", Content: "Hello!"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "gemini", resp.ProviderID)
	assert.Equal(t, int64(11), resp.Usage.InputTokens)
	assert.Equal(t, int64(14), resp.Usage.OutputTokens)
	assert.Equal(t, int64(25), resp.Usage.TotalTokens)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Gemini!", resp.Choices[0].Message.Content)
}
