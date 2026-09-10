// Package handlers contains HTTP request handlers.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/auth"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/ratelimit"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/routing"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/tenant"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/usage"
)

// ProviderStore looks up ProviderConnection records for a project.
type ProviderConnectionStore interface {
	GetByProjectAndProvider(ctx context.Context, projectID string, p domain.ProviderID) (*domain.ProviderConnection, error)
}

// RoutingPolicyStore fetches the active routing policy for a project.
type RoutingPolicyStore interface {
	GetActiveByProject(ctx context.Context, projectID string) (*domain.RoutingPolicy, error)
}

// RequestRecordStore persists per-request audit records.
type RequestRecordStore interface {
	Create(ctx context.Context, record *domain.RequestRecord) error
}

// ChatHandler handles POST /v1/chat/completions.
type ChatHandler struct {
	validator        *auth.Validator
	tenantResolver   *tenant.Resolver
	rateLimiter      *ratelimit.Limiter
	providerReg      *provider.Registry
	routingEngine    *routing.Engine
	policyStore      RoutingPolicyStore
	requestStore     RequestRecordStore
	usageProducer    usage.Producer
	metrics          *observability.Metrics
	logger           *zap.Logger
	defaultRateLimit ratelimit.Limit
}

// ChatHandlerConfig holds constructor arguments for ChatHandler.
type ChatHandlerConfig struct {
	Validator        *auth.Validator
	TenantResolver   *tenant.Resolver
	RateLimiter      *ratelimit.Limiter
	ProviderReg      *provider.Registry
	PolicyStore      RoutingPolicyStore
	RequestStore     RequestRecordStore
	UsageProducer    usage.Producer
	Metrics          *observability.Metrics
	Logger           *zap.Logger
	DefaultRateLimit ratelimit.Limit
}

// NewChatHandler creates a ChatHandler with all dependencies injected.
func NewChatHandler(cfg ChatHandlerConfig) *ChatHandler {
	return &ChatHandler{
		validator:        cfg.Validator,
		tenantResolver:   cfg.TenantResolver,
		rateLimiter:      cfg.RateLimiter,
		providerReg:      cfg.ProviderReg,
		routingEngine:    routing.NewEngine(cfg.ProviderReg),
		policyStore:      cfg.PolicyStore,
		requestStore:     cfg.RequestStore,
		usageProducer:    cfg.UsageProducer,
		metrics:          cfg.Metrics,
		logger:           cfg.Logger,
		defaultRateLimit: cfg.DefaultRateLimit,
	}
}

// chatCompletionRequest is the OpenAI-compatible request body.
type chatCompletionRequest struct {
	Model       string            `json:"model"`
	Messages    []messageRequest  `json:"messages"`
	Stream      bool              `json:"stream"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
	Temperature *float32          `json:"temperature,omitempty"`
	TopP        *float32          `json:"top_p,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	User        string            `json:"user,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type messageRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ServeHTTP handles the chat completions endpoint.
func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":{"message":"method not allowed","code":"method_not_allowed"}}`, http.StatusMethodNotAllowed)
		return
	}

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = domain.NewRequestRecord("", "").RequestID
	}
	ctx := r.Context()
	log := observability.WithRequestID(h.logger, requestID)

	// 1. Authenticate
	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		writeError(w, requestID, domain.ErrUnauthorized)
		return
	}

	apiKey, err := h.validator.Validate(ctx, bearerToken)
	if err != nil {
		writeError(w, requestID, err)
		return
	}

	// 2. Resolve tenant
	tc, err := h.tenantResolver.Resolve(ctx, apiKey)
	if err != nil {
		writeError(w, requestID, err)
		return
	}

	// 3. Check scope
	if !apiKey.HasScope(domain.ScopeInferenceWrite) && !apiKey.HasScope(domain.ScopeAdmin) {
		writeError(w, requestID, domain.ErrForbidden)
		return
	}

	// 4. Rate limit (per project)
	result, err := h.rateLimiter.Check(ctx, ratelimit.ScopeProject, tc.Project.ID, h.defaultRateLimit)
	if err != nil {
		log.Warn("rate limit check error", zap.Error(err))
	} else if !result.Allowed {
		h.metrics.RateLimitRejectionsTotal.WithLabelValues("project").Inc()
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", result.Limit))
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetAt.Unix()))
		writeError(w, requestID, domain.ErrRateLimitExceeded)
		return
	}

	// 5. Decode request body
	var reqBody chatCompletionRequest
	if err := decodeJSON(r, &reqBody); err != nil {
		writeError(w, requestID, err)
		return
	}
	if len(reqBody.Messages) == 0 {
		writeError(w, requestID, domain.New(domain.CodeBadRequest, "messages is required and must not be empty"))
		return
	}

	// 6. Build provider request
	provReq := &provider.ChatRequest{
		Model:       reqBody.Model,
		Stream:      reqBody.Stream,
		MaxTokens:   reqBody.MaxTokens,
		Temperature: reqBody.Temperature,
		TopP:        reqBody.TopP,
		Stop:        reqBody.Stop,
		User:        reqBody.User,
		RequestID:   requestID,
		ProjectID:   tc.Project.ID,
	}

	// Apply Caveman mode
	cavemanEnabled := true
	if val, ok := reqBody.Metadata["caveman"]; ok && (val == "false" || val == "off" || val == "0") {
		cavemanEnabled = false
	}
	if r.Header.Get("X-Nexus-Caveman") == "false" || r.Header.Get("X-Nexus-Caveman") == "off" {
		cavemanEnabled = false
	}

	if cavemanEnabled {
		cavemanPrompt := "Respond directly and concisely. No fluff, no filler, no introductory pleasantries, no conversational padding. Optimize for brevity and token savings."
		hasSystem := false
		for _, m := range reqBody.Messages {
			if strings.ToLower(m.Role) == "system" {
				provReq.Messages = append(provReq.Messages, provider.Message{
					Role:    m.Role,
					Content: m.Content + "\n" + cavemanPrompt,
				})
				hasSystem = true
			} else {
				provReq.Messages = append(provReq.Messages, provider.Message{
					Role:    m.Role,
					Content: m.Content,
				})
			}
		}
		if !hasSystem {
			provReq.Messages = append([]provider.Message{{Role: "system", Content: cavemanPrompt}}, provReq.Messages...)
		}
	} else {
		for _, m := range reqBody.Messages {
			provReq.Messages = append(provReq.Messages, provider.Message{
				Role:    m.Role,
				Content: m.Content,
			})
		}
	}

	// 7. Resolve routing policy
	policy, err := h.policyStore.GetActiveByProject(ctx, tc.Project.ID)
	if err != nil {
		log.Warn("no active routing policy, using first registered provider", zap.Error(err))
	}

	// 8. Select provider via Routing Engine
	selectedProvider, decision, err := h.routingEngine.Route(provReq, policy)
	if err != nil {
		writeError(w, requestID, err)
		return
	}

	// Set decision headers on response for client observability
	w.Header().Set("X-Nexus-Route-Model", decision.SelectedModel)
	w.Header().Set("X-Nexus-Route-Provider", decision.ProviderID)
	w.Header().Set("X-Nexus-Route-Tier", decision.Tier)
	w.Header().Set("X-Nexus-Route-Complexity", fmt.Sprintf("%.2f", decision.Complexity))
	w.Header().Set("X-Nexus-Route-Intent", decision.Intent)
	w.Header().Set("X-Nexus-Route-Rationale", decision.Rationale)

	// 9. Create request record
	record := domain.NewRequestRecord(requestID, tc.Project.ID)
	if policy != nil {
		record.RoutingStrategy = policy.Strategy
	}

	// 10. Execute request
	provReq.Model = decision.SelectedModel
	start := time.Now()

	if reqBody.Stream {
		h.handleStream(w, r, ctx, selectedProvider, decision.SelectedModel, provReq, record, start, requestID, tc.Project.ID, decision)
		return
	}

	resp, err := selectedProvider.Chat(ctx, provReq)
	latencyMS := time.Since(start).Milliseconds()

	if err != nil {
		record.Fail(domain.CodeOf(err), latencyMS)
		h.saveAndEmit(ctx, record, tc.Project.ID)
		h.metrics.ProviderRequestsTotal.WithLabelValues(selectedProvider.ID(), decision.SelectedModel, "error").Inc()
		writeError(w, requestID, err)
		return
	}

	// 11. Record metrics and complete the record
	cost := estimateCost(decision.SelectedModel, resp.Usage)
	record.Complete(domain.ProviderID(selectedProvider.ID()), decision.SelectedModel, domain.TokenUsage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		TotalTokens:  resp.Usage.TotalTokens,
	}, latencyMS, cost)

	h.metrics.ProviderRequestsTotal.WithLabelValues(selectedProvider.ID(), decision.SelectedModel, "success").Inc()
	h.metrics.ProviderRequestDuration.WithLabelValues(selectedProvider.ID(), decision.SelectedModel).Observe(float64(latencyMS) / 1000)
	h.metrics.LLMTokensTotal.WithLabelValues("input", selectedProvider.ID(), decision.SelectedModel).Add(float64(resp.Usage.InputTokens))
	h.metrics.LLMTokensTotal.WithLabelValues("output", selectedProvider.ID(), decision.SelectedModel).Add(float64(resp.Usage.OutputTokens))

	// 12. Save record and emit usage event asynchronously
	h.saveAndEmit(ctx, record, tc.Project.ID)

	// 13. Respond — OpenAI-compatible shape with nexus routing metadata
	writeJSON(w, http.StatusOK, toOpenAIResponse(resp, requestID, decision))
}

// handleStream writes an SSE streaming response.
func (h *ChatHandler) handleStream(
	w http.ResponseWriter, r *http.Request, ctx context.Context,
	prov provider.Provider, model string, req *provider.ChatRequest,
	record *domain.RequestRecord, start time.Time,
	requestID, projectID string, decision *routing.Decision,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, requestID, domain.New(domain.CodeInternal, "streaming unsupported by transport"))
		return
	}

	chunkCh, err := prov.StreamChat(ctx, req)
	if err != nil {
		record.Fail(domain.CodeOf(err), time.Since(start).Milliseconds())
		h.saveAndEmit(ctx, record, projectID)
		h.metrics.ProviderRequestsTotal.WithLabelValues(prov.ID(), model, "error").Inc()
		errObj := map[string]interface{}{"error": map[string]string{"message": err.Error(), "code": string(domain.CodeOf(err))}}
		data, _ := json.Marshal(errObj)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return
	}

	var finalUsage *provider.TokenUsage
	for chunk := range chunkCh {
		if chunk.Err != nil {
			h.logger.Warn("stream chunk error", zap.Error(chunk.Err), zap.String("request_id", requestID))
			break
		}
		if chunk.Usage != nil {
			finalUsage = chunk.Usage
		}

		data, err := marshalSSEChunk(chunk, requestID)
		if err != nil {
			h.logger.Warn("failed to marshal chunk", zap.Error(err))
			continue
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	latencyMS := time.Since(start).Milliseconds()
	if finalUsage != nil {
		cost := estimateCost(model, *finalUsage)
		record.Complete(domain.ProviderID(prov.ID()), model, domain.TokenUsage{
			InputTokens:  finalUsage.InputTokens,
			OutputTokens: finalUsage.OutputTokens,
			TotalTokens:  finalUsage.TotalTokens,
		}, latencyMS, cost)
	} else {
		record.Complete(domain.ProviderID(prov.ID()), model, domain.TokenUsage{}, latencyMS, 0)
	}

	h.saveAndEmit(ctx, record, projectID)
}

// saveAndEmit persists the request record and emits a usage event.
func (h *ChatHandler) saveAndEmit(ctx context.Context, record *domain.RequestRecord, projectID string) {
	go func() {
		bgCtx := context.Background()
		if err := h.requestStore.Create(bgCtx, record); err != nil {
			h.logger.Warn("failed to save request record", zap.Error(err), zap.String("request_id", record.RequestID))
		}
		event := domain.NewUsageEvent(record)
		if err := h.usageProducer.Emit(bgCtx, event); err != nil {
			h.logger.Warn("failed to emit usage event", zap.Error(err), zap.String("event_id", event.EventID))
		}
	}()
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
		return auth[len(prefix):]
	}
	return ""
}

func toOpenAIResponse(resp *provider.ChatResponse, requestID string, decision *routing.Decision) map[string]interface{} {
	choices := make([]map[string]interface{}, 0, len(resp.Choices))
	for _, c := range resp.Choices {
		choices = append(choices, map[string]interface{}{
			"index":         c.Index,
			"message":       map[string]string{"role": c.Message.Role, "content": c.Message.Content},
			"finish_reason": c.FinishReason,
		})
	}

	res := map[string]interface{}{
		"id":      resp.ID,
		"object":  "chat.completion",
		"created": resp.Created,
		"model":   resp.Model,
		"choices": choices,
		"usage": map[string]int64{
			"prompt_tokens":     resp.Usage.InputTokens,
			"completion_tokens": resp.Usage.OutputTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		},
		"x_request_id": requestID,
	}

	if decision != nil {
		res["nexus_routing"] = map[string]interface{}{
			"requested_model":  decision.RequestedModel,
			"selected_model":   decision.SelectedModel,
			"provider":         decision.ProviderID,
			"tier":             decision.Tier,
			"complexity_score": decision.Complexity,
			"intent":           decision.Intent,
			"rationale":        decision.Rationale,
			"context_chars":    decision.ContextChars,
		}
	}

	return res
}

func marshalSSEChunk(chunk provider.ChatChunk, requestID string) ([]byte, error) {
	choices := make([]map[string]interface{}, 0, len(chunk.Choices))
	for _, c := range chunk.Choices {
		choices = append(choices, map[string]interface{}{
			"index":         c.Index,
			"delta":         map[string]string{"role": c.Delta.Role, "content": c.Delta.Content},
			"finish_reason": nilIfEmpty(c.FinishReason),
		})
	}
	obj := map[string]interface{}{
		"id":      chunk.ID,
		"object":  "chat.completion.chunk",
		"created": chunk.Created,
		"model":   chunk.Model,
		"choices": choices,
	}
	return json.Marshal(obj)
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func estimateCost(model string, usage provider.TokenUsage) float64 {
	type pricing struct{ input, output float64 }
	prices := map[string]pricing{
		"gpt-4o":                    {0.0025, 0.01},   // per 1K tokens
		"gpt-4o-mini":               {0.00015, 0.0006},
		"claude-3-5-sonnet-20241022": {0.003, 0.015},
		"claude-3-haiku-20240307":   {0.00025, 0.00125},
		"gemini-3.6-flash":           {0.000075, 0.0003},
		"gemini-2.0-flash":           {0.000075, 0.0003},
		"gemini-2.5-pro":             {0.00125, 0.005},
		"gemini-1.5-pro":             {0.00125, 0.005},
	}
	p, ok := prices[model]
	if !ok {
		return 0
	}
	inputCost := float64(usage.InputTokens) / 1000 * p.input
	outputCost := float64(usage.OutputTokens) / 1000 * p.output
	return inputCost + outputCost
}