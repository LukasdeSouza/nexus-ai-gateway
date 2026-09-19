// Package gemini provides the Google Gemini API adapter implementing provider.Provider.
package gemini

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

// Adapter implements provider.Provider for Google Gemini.
type Adapter struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewAdapter creates a Gemini adapter instance.
func NewAdapter(baseURL, apiKey string) *Adapter {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
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
	return "gemini"
}

// Capabilities returns standard capabilities for Gemini models.
func (a *Adapter) Capabilities(_ context.Context) (*provider.Capabilities, error) {
	return &provider.Capabilities{
		Chat:                 true,
		Streaming:            true,
		FunctionCall:         true,
		Vision:               true,
		MaxContextTokens:     1000000,
		InputPricePerMToken:  0.075,
		OutputPricePerMToken: 0.30,
	}, nil
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *struct {
		Parts []geminiPart `json:"parts"`
	} `json:"system_instruction,omitempty"`
	GenerationConfig *struct {
		MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
		Temperature     *float32 `json:"temperature,omitempty"`
		TopP            *float32 `json:"topP,omitempty"`
	} `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Role  string       `json:"role"`
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
		Index        int    `json:"index"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int64 `json:"promptTokenCount"`
		CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		TotalTokenCount      int64 `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (a *Adapter) buildRequest(req *provider.ChatRequest) geminiRequest {
	var contents []geminiContent
	var systemParts []geminiPart

	for _, m := range req.Messages {
		role := strings.ToLower(m.Role)
		if role == "system" {
			systemParts = append(systemParts, geminiPart{Text: m.Content})
		} else {
			if role == "assistant" {
				role = "model"
			}
			contents = append(contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: m.Content}},
			})
		}
	}

	gReq := geminiRequest{
		Contents: contents,
	}

	if len(systemParts) > 0 {
		gReq.SystemInstruction = &struct {
			Parts []geminiPart `json:"parts"`
		}{Parts: systemParts}
	}

	if req.MaxTokens != nil || req.Temperature != nil || req.TopP != nil {
		gReq.GenerationConfig = &struct {
			MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
			Temperature     *float32 `json:"temperature,omitempty"`
			TopP            *float32 `json:"topP,omitempty"`
		}{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
			TopP:            req.TopP,
		}
	}

	return gReq
}

// Chat executes a non-streaming completion request against Gemini.
func (a *Adapter) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	gReq := a.buildRequest(req)
	bodyBytes, err := json.Marshal(gReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/models/%s:generateContent", a.baseURL, req.Model)
	apiKey := a.resolveAPIKey(req)
	if apiKey != "" {
		endpoint += "?key=" + apiKey
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
		var errResp geminiResponse
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

	var gResp geminiResponse
	if err := json.Unmarshal(respBytes, &gResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	res := &provider.ChatResponse{
		ID:         fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
		Object:     "chat.completion",
		Created:    time.Now().Unix(),
		Model:      req.Model,
		ProviderID: a.ID(),
		Usage: provider.TokenUsage{
			InputTokens:  gResp.UsageMetadata.PromptTokenCount,
			OutputTokens: gResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  gResp.UsageMetadata.TotalTokenCount,
		},
		RequestID: req.RequestID,
	}

	for _, cand := range gResp.Candidates {
		var text string
		for _, part := range cand.Content.Parts {
			text += part.Text
		}
		finishReason := strings.ToLower(cand.FinishReason)
		if finishReason == "stop" {
			finishReason = "stop"
		}
		res.Choices = append(res.Choices, provider.Choice{
			Index: cand.Index,
			Message: provider.Message{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: finishReason,
		})
	}

	return res, nil
}

// StreamChat starts a streaming completion request against Gemini.
func (a *Adapter) StreamChat(ctx context.Context, req *provider.ChatRequest) (<-chan provider.ChatChunk, error) {
	gReq := a.buildRequest(req)
	bodyBytes, err := json.Marshal(gReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/models/%s:streamGenerateContent?alt=sse", a.baseURL, req.Model)
	streamAPIKey := a.resolveAPIKey(req)
	if streamAPIKey != "" {
		endpoint += "&key=" + streamAPIKey
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
			var gResp geminiResponse
			if err := json.Unmarshal([]byte(payload), &gResp); err != nil {
				continue
			}

			for _, cand := range gResp.Candidates {
				var text string
				for _, part := range cand.Content.Parts {
					text += part.Text
				}
				if text != "" {
					chunk := provider.ChatChunk{
						ID:         fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
						Object:     "chat.completion.chunk",
						Created:    time.Now().Unix(),
						Model:      req.Model,
						ProviderID: a.ID(),
						Choices: []provider.StreamChoice{
							{
								Index: cand.Index,
								Delta: provider.MessageDelta{
									Content: text,
								},
								FinishReason: strings.ToLower(cand.FinishReason),
							},
						},
					}
					if gResp.UsageMetadata.TotalTokenCount > 0 {
						chunk.Usage = &provider.TokenUsage{
							InputTokens:  gResp.UsageMetadata.PromptTokenCount,
							OutputTokens: gResp.UsageMetadata.CandidatesTokenCount,
							TotalTokens:  gResp.UsageMetadata.TotalTokenCount,
						}
					}

					select {
					case <-ctx.Done():
						return
					case out <- chunk:
					}
				}
			}
		}
	}()

	return out, nil
}

func (a *Adapter) resolveAPIKey(req *provider.ChatRequest) string {
	if req != nil && req.CustomAPIKeys != nil {
		if k, ok := req.CustomAPIKeys["gemini"]; ok && k != "" {
			return k
		}
	}
	return a.apiKey
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
