// Package openai provides the OpenAI adapter implementing provider.Provider.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
)

// Adapter implements provider.Provider for OpenAI.
type Adapter struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewAdapter creates an OpenAI adapter instance.
func NewAdapter(baseURL, apiKey string) *Adapter {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &Adapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// ID returns the provider name.
func (a *Adapter) ID() string {
	return "openai"
}

// Capabilities returns standard capabilities for OpenAI models.
func (a *Adapter) Capabilities(_ context.Context) (*provider.Capabilities, error) {
	return &provider.Capabilities{
		Chat:                 true,
		Streaming:            true,
		FunctionCall:         true,
		Vision:               true,
		MaxContextTokens:     128000,
		InputPricePerMToken:  2.50,
		OutputPricePerMToken: 10.00,
	}, nil
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float32        `json:"temperature,omitempty"`
	TopP        *float32        `json:"top_p,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	User        string          `json:"user,omitempty"`
}

type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Chat executes a non-streaming completion request against OpenAI.
func (a *Adapter) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	oReq := openAIRequest{
		Model:       req.Model,
		Stream:      false,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
		User:        req.User,
	}
	for _, m := range req.Messages {
		oReq.Messages = append(oReq.Messages, openAIMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	bodyBytes, err := json.Marshal(oReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	if req.RequestID != "" {
		httpReq.Header.Set("X-Request-ID", req.RequestID)
	}

	httpResp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{
			StatusCode: http.StatusGatewayTimeout,
			Message:    err.Error(),
			Retryable:  true,
			Provider:   a.ID(),
		}
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var errResp openAIResponse
		_ = json.Unmarshal(respBytes, &errResp)
		msg := string(respBytes)
		if errResp.Error != nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}
		return nil, &provider.ProviderError{
			StatusCode: httpResp.StatusCode,
			Message:    msg,
			Retryable:  isStatusCodeRetryable(httpResp.StatusCode),
			Provider:   a.ID(),
		}
	}

	var oResp openAIResponse
	if err := json.Unmarshal(respBytes, &oResp); err != nil {
		return nil, fmt.Errorf("failed to decode response json: %w", err)
	}

	res := &provider.ChatResponse{
		ID:         oResp.ID,
		Object:     oResp.Object,
		Created:    oResp.Created,
		Model:      oResp.Model,
		ProviderID: a.ID(),
		Usage: provider.TokenUsage{
			InputTokens:  oResp.Usage.PromptTokens,
			OutputTokens: oResp.Usage.CompletionTokens,
			TotalTokens:  oResp.Usage.TotalTokens,
		},
		RequestID: req.RequestID,
	}

	for _, c := range oResp.Choices {
		res.Choices = append(res.Choices, provider.Choice{
			Index: c.Index,
			Message: provider.Message{
				Role:    c.Message.Role,
				Content: c.Message.Content,
			},
			FinishReason: c.FinishReason,
		})
	}

	return res, nil
}

// StreamChat starts a streaming completion request against OpenAI.
func (a *Adapter) StreamChat(ctx context.Context, req *provider.ChatRequest) (<-chan provider.ChatChunk, error) {
	oReq := openAIRequest{
		Model:       req.Model,
		Stream:      true,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
		User:        req.User,
	}
	for _, m := range req.Messages {
		oReq.Messages = append(oReq.Messages, openAIMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	bodyBytes, err := json.Marshal(oReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal streaming request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	if req.RequestID != "" {
		httpReq.Header.Set("X-Request-ID", req.RequestID)
	}

	httpResp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{
			StatusCode: http.StatusGatewayTimeout,
			Message:    err.Error(),
			Retryable:  true,
			Provider:   a.ID(),
		}
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		errBytes, _ := io.ReadAll(httpResp.Body)
		return nil, &provider.ProviderError{
			StatusCode: httpResp.StatusCode,
			Message:    string(errBytes),
			Retryable:  isStatusCodeRetryable(httpResp.StatusCode),
			Provider:   a.ID(),
		}
	}

	out := make(chan provider.ChatChunk, 32)
	go func() {
		defer close(out)
		defer httpResp.Body.Close()

		reader := bufio.NewReader(httpResp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF && ctx.Err() == nil {
					out <- provider.ChatChunk{Err: err}
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}

			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				return
			}

			var chunk struct {
				ID      string `json:"id"`
				Object  string `json:"object"`
				Created int64  `json:"created"`
				Model   string `json:"model"`
				Choices []struct {
					Index int `json:"index"`
					Delta struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage *struct {
					PromptTokens     int64 `json:"prompt_tokens"`
					CompletionTokens int64 `json:"completion_tokens"`
					TotalTokens      int64 `json:"total_tokens"`
				} `json:"usage,omitempty"`
			}

			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				continue
			}

			cc := provider.ChatChunk{
				ID:         chunk.ID,
				Object:     chunk.Object,
				Created:    chunk.Created,
				Model:      chunk.Model,
				ProviderID: a.ID(),
			}
			for _, ch := range chunk.Choices {
				cc.Choices = append(cc.Choices, provider.StreamChoice{
					Index: ch.Index,
					Delta: provider.MessageDelta{
						Role:    ch.Delta.Role,
						Content: ch.Delta.Content,
					},
					FinishReason: ch.FinishReason,
				})
			}
			if chunk.Usage != nil {
				cc.Usage = &provider.TokenUsage{
					InputTokens:  chunk.Usage.PromptTokens,
					OutputTokens: chunk.Usage.CompletionTokens,
					TotalTokens:  chunk.Usage.TotalTokens,
				}
			}

			select {
			case <-ctx.Done():
				return
			case out <- cc:
			}
		}
	}()

	return out, nil
}

func isStatusCodeRetryable(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
