package openai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider/openai"
)

func TestOpenAIAdapter_Chat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 1677652288,
			"model": "gpt-4o",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "Hello from OpenAI!"},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 9,
				"completion_tokens": 12,
				"total_tokens": 21
			}
		}`))
	}))
	defer server.Close()

	adapter := openai.NewAdapter(server.URL, "test-key")
	resp, err := adapter.Chat(context.Background(), &provider.ChatRequest{
		Model: "gpt-4o",
		Messages: []provider.Message{
			{Role: "user", Content: "Hello!"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", resp.ID)
	assert.Equal(t, "openai", resp.ProviderID)
	assert.Equal(t, int64(9), resp.Usage.InputTokens)
	assert.Equal(t, int64(12), resp.Usage.OutputTokens)
	assert.Equal(t, int64(21), resp.Usage.TotalTokens)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from OpenAI!", resp.Choices[0].Message.Content)
}

func TestOpenAIAdapter_Chat_RetryableErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		retryable  bool
	}{
		{"RateLimited 429", http.StatusTooManyRequests, true},
		{"InternalError 500", http.StatusInternalServerError, true},
		{"ServiceUnavailable 503", http.StatusServiceUnavailable, true},
		{"BadRequest 400", http.StatusBadRequest, false},
		{"Unauthorized 401", http.StatusUnauthorized, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"error": {"message": "error occurred"}}`))
			}))
			defer server.Close()

			adapter := openai.NewAdapter(server.URL, "test-key")
			_, err := adapter.Chat(context.Background(), &provider.ChatRequest{Model: "gpt-4o"})

			require.Error(t, err)
			var pe *provider.ProviderError
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, tt.statusCode, pe.StatusCode)
			assert.Equal(t, tt.retryable, pe.Retryable)
		})
	}
}
