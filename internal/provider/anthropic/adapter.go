// Package anthropic provides the Anthropic Messages API adapter implementing provider.Provider.
package anthropic

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

// Adapter implements provider.Provider for Anthropic.
type Adapter struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewAdapter creates an Anthropic adapter instance.
func NewAdapter(baseURL, apiKey string) *Adapter {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
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
	return "anthropic"
}

// Capabilities returns standard capabilities for Anthropic models.
func (a *Adapter) Capabilities(_ context.Context) (*provider.Capabilities, error) {
	return &provider.Capabilities{
		Chat:                 true,
		Streaming:            true,
		FunctionCall:         true,
		Vision:               true,
		MaxContextTokens:     200000,
		InputPricePerMToken:  3.00,
		OutputPricePerMToken: 15.00,
	}, nil
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float32           `json:"temperature,omitempty"`
	TopP        *float32           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *Adapter) buildRequest(req *provider.ChatRequest, stream bool) anthropicRequest {
	maxTokens := 4096
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	var systemPrompt strings.Builder
	var msgs []anthropicMessage
	for _, m := range req.Messages {
		if strings.ToLower(m.Role) == "system" {
			if systemPrompt.Len() > 0 {
				systemPrompt.WriteString("\n")
			}
			systemPrompt.WriteString(m.Content)
		} else {
			msgs = append(msgs, anthropicMessage{
				Role:    m.Role,
				Content: m.Content,
			})
		}
	}

	return anthropicRequest{
		Model:       req.Model,
		Messages:    msgs,
		System:      systemPrompt.String(),
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      stream,
	}
}

// Chat executes a non-streaming completion request against Anthropic.
func (a *Adapter) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	antReq := a.buildRequest(req, false)

	bodyBytes, err := json.Marshal(antReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if a.apiKey != "" {
		httpReq.Header.Set("x-api-key", a.apiKey)
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
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var errResp anthropicResponse
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

	var antResp anthropicResponse
	if err := json.Unmarshal(respBytes, &antResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var contentBuilder strings.Builder
	for _, c := range antResp.Content {
		if c.Type == "text" {
			contentBuilder.WriteString(c.Text)
		}
	}

	totalTokens := antResp.Usage.InputTokens + antResp.Usage.OutputTokens

	finishReason := antResp.StopReason
	if finishReason == "end_turn" {
		finishReason = "stop"
	}

	return &provider.ChatResponse{
		ID:         antResp.ID,
		Object:     "chat.completion",
		Created:    time.Now().Unix(),
		Model:      antResp.Model,
		ProviderID: a.ID(),
		Choices: []provider.Choice{
			{
				Index: 0,
				Message: provider.Message{
					Role:    "assistant",
					Content: contentBuilder.String(),
				},
				FinishReason: finishReason,
			},
		},
		Usage: provider.TokenUsage{
			InputTokens:  antResp.Usage.InputTokens,
			OutputTokens: antResp.Usage.OutputTokens,
			TotalTokens:  totalTokens,
		},
		RequestID: req.RequestID,
	}, nil
}

// StreamChat starts a streaming completion request against Anthropic.
func (a *Adapter) StreamChat(ctx context.Context, req *provider.ChatRequest) (<-chan provider.ChatChunk, error) {
	antReq := a.buildRequest(req, true)

	bodyBytes, err := json.Marshal(antReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if a.apiKey != "" {
		httpReq.Header.Set("x-api-key", a.apiKey)
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
		var msgID, model string
		var inputTokens, outputTokens int64

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

			var event struct {
				Type    string `json:"type"`
				Message struct {
					ID    string `json:"id"`
					Model string `json:"model"`
					Usage struct {
						InputTokens int64 `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
				Usage *struct {
					OutputTokens int64 `json:"output_tokens"`
				} `json:"usage,omitempty"`
			}

			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				continue
			}

			switch event.Type {
			case "message_start":
				msgID = event.Message.ID
				model = event.Message.Model
				inputTokens = event.Message.Usage.InputTokens
			case "content_block_delta":
				if event.Delta.Text != "" {
					chunk := provider.ChatChunk{
						ID:         msgID,
						Object:     "chat.completion.chunk",
						Created:    time.Now().Unix(),
						Model:      model,
						ProviderID: a.ID(),
						Choices: []provider.StreamChoice{
							{
								Index: 0,
								Delta: provider.MessageDelta{
									Content: event.Delta.Text,
								},
							},
						},
					}
					select {
					case <-ctx.Done():
						return
					case out <- chunk:
					}
				}
			case "message_delta":
				if event.Usage != nil {
					outputTokens = event.Usage.OutputTokens
				}
			case "message_stop":
				finalChunk := provider.ChatChunk{
					ID:         msgID,
					Object:     "chat.completion.chunk",
					Created:    time.Now().Unix(),
					Model:      model,
					ProviderID: a.ID(),
					Usage: &provider.TokenUsage{
						InputTokens:  inputTokens,
						OutputTokens: outputTokens,
						TotalTokens:  inputTokens + outputTokens,
					},
					Choices: []provider.StreamChoice{
						{
							Index:        0,
							FinishReason: "stop",
						},
					},
				}
				select {
				case <-ctx.Done():
				case out <- finalChunk:
				}
				return
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
