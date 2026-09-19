// Package provider defines the upstream AI provider abstraction and adapters.
package provider

import (
	"context"
	"fmt"
)

// Provider is the internal interface that all provider adapters implement.
type Provider interface {
	ID() string
	Capabilities(ctx context.Context) (*Capabilities, error)
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req *ChatRequest) (<-chan ChatChunk, error)
}

// ChatRequest is the normalized request passed to provider adapters.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Temperature *float32  `json:"temperature,omitempty"`
	TopP        *float32  `json:"top_p,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	User        string    `json:"user,omitempty"`
	RequestID     string            `json:"request_id,omitempty"`
	ProjectID     string            `json:"project_id,omitempty"`
	CustomAPIKeys map[string]string `json:"custom_api_keys,omitempty"`
}

// Message represents a single conversational turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TokenUsage holds token counts for an interaction.
type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// ChatResponse is the normalized response from a non-streaming provider call.
type ChatResponse struct {
	ID         string     `json:"id"`
	Object     string     `json:"object"`
	Created    int64      `json:"created"`
	Model      string     `json:"model"`
	ProviderID string     `json:"provider_id"`
	Choices    []Choice   `json:"choices"`
	Usage      TokenUsage `json:"usage"`
	RequestID  string     `json:"request_id,omitempty"`
}

// Choice represents a completion choice in ChatResponse.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// ChatChunk represents a single token or delta in a streaming completion.
type ChatChunk struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"`
	Created    int64          `json:"created"`
	Model      string         `json:"model"`
	ProviderID string         `json:"provider_id"`
	Choices    []StreamChoice `json:"choices"`
	Usage      *TokenUsage    `json:"usage,omitempty"`
	Err        error          `json:"-"`
}

// StreamChoice represents a delta inside a streaming chunk.
type StreamChoice struct {
	Index        int          `json:"index"`
	Delta        MessageDelta `json:"delta"`
	FinishReason string       `json:"finish_reason,omitempty"`
}

// MessageDelta holds partial text or role update.
type MessageDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// Capabilities declares model and adapter capabilities.
type Capabilities struct {
	Chat                 bool    `json:"chat"`
	Streaming            bool    `json:"streaming"`
	FunctionCall         bool    `json:"function_call"`
	Vision               bool    `json:"vision"`
	MaxContextTokens     int     `json:"max_context_tokens"`
	InputPricePerMToken  float64 `json:"input_price_per_mtoken"`
	OutputPricePerMToken float64 `json:"output_price_per_mtoken"`
}

// ProviderError captures upstream HTTP status and failure details.
type ProviderError struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	Provider   string `json:"provider"`
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider error [%s, HTTP %d]: %s", e.Provider, e.StatusCode, e.Message)
}

// IsRetryable determines if a provider error should trigger retry or failover.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if pe, ok := err.(*ProviderError); ok {
		return pe.Retryable
	}
	return false
}
