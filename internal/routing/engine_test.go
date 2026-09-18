package routing

import (
	"context"
	"testing"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
	"github.com/stretchr/testify/assert"
)

type mockProvider struct {
	id string
}

func (m *mockProvider) ID() string { return m.id }
func (m *mockProvider) Capabilities(ctx context.Context) (*provider.Capabilities, error) {
	return &provider.Capabilities{Chat: true}, nil
}
func (m *mockProvider) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{
		ID: "mock",
		Choices: []provider.Choice{{
			Index:   0,
			Message: provider.Message{Role: "assistant", Content: "mock answer"},
		}},
	}, nil
}
func (m *mockProvider) StreamChat(ctx context.Context, req *provider.ChatRequest) (<-chan provider.ChatChunk, error) {
	return nil, nil
}

func TestRoutePlanPresets(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&mockProvider{id: "gemini"})
	reg.Register(&mockProvider{id: "anthropic"})
	reg.Register(&mockProvider{id: "openai"})

	engine := NewEngine(reg)

	// 1. Explore preset
	req := &provider.ChatRequest{
		Model: "explore",
		Messages: []provider.Message{
			{Role: "user", Content: "Where is the auth handler defined?"},
		},
	}
	dec, err := engine.RoutePlan(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "explore", dec.Preset)
	assert.Equal(t, "fast", dec.Tier)
	assert.NotEmpty(t, dec.Candidates)
	assert.Equal(t, "gemini-3.6-flash", dec.SelectedModel)

	// 2. Build preset
	req = &provider.ChatRequest{
		Model: "build",
		Messages: []provider.Message{
			{Role: "user", Content: "Refactor user authentication to support passkeys"},
		},
	}
	dec, err = engine.RoutePlan(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "build", dec.Preset)
	assert.Equal(t, "smart", dec.Tier)

	// 3. Reason preset
	req = &provider.ChatRequest{
		Model: "reason",
		Messages: []provider.Message{
			{Role: "user", Content: "Analyze distributed deadlock and race condition"},
		},
	}
	dec, err = engine.RoutePlan(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "reason", dec.Preset)
	assert.Equal(t, "smart", dec.Tier)
	assert.GreaterOrEqual(t, dec.Confidence, 90)

	// 4. Auto dynamic routing on complex prompt
	req = &provider.ChatRequest{
		Model: "auto",
		Messages: []provider.Message{
			{Role: "user", Content: "Investigate architecture and microservices security audit for authentication"},
		},
	}
	dec, err = engine.RoutePlan(req, nil)
	assert.NoError(t, err)
	assert.True(t, dec.Preset == "reason" || dec.Preset == "build")
	assert.GreaterOrEqual(t, dec.Complexity, 0.7)
}