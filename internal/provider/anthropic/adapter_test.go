package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider/anthropic"
)

func TestAnthropicAdapter_Chat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "test-ant-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Equal(t, "You are a helpful assistant.", reqBody["system"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_01XFDUDYJgAACzvnptvVoYEL",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "Hello from Claude!"}],
			"model": "claude-3-5-sonnet-20241022",
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 15,
				"output_tokens": 8
			}
		}`))
	}))
	defer server.Close()

	adapter := anthropic.NewAdapter(server.URL, "test-ant-key")
	resp, err := adapter.Chat(context.Background(), &provider.ChatRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []provider.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello!"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "msg_01XFDUDYJgAACzvnptvVoYEL", resp.ID)
	assert.Equal(t, "anthropic", resp.ProviderID)
	assert.Equal(t, int64(15), resp.Usage.InputTokens)
	assert.Equal(t, int64(8), resp.Usage.OutputTokens)
	assert.Equal(t, int64(23), resp.Usage.TotalTokens)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Claude!", resp.Choices[0].Message.Content)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
}
