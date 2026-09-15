// Package handlers contains HTTP request handlers.
package handlers

import (
	"context"
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

// ChatHandler handles POST /v1/chat/completions with coding agent instructions and failover.
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

const codingAgentSystemPrompt = `You are Nexus, an elite AI coding assistant and agent.
When the user asks you to modify, edit, or create files, output your proposed changes using the exact block formats below so the Nexus CLI can inspect diffs and apply them safely to disk:

1. To edit an existing file with surgical precision (Search/Replace):
` + "```edit:path/to/file.ext\n<<<<<<< SEARCH\nexact original code snippet to replace\n=======\nexact new code snippet\n>>>>>>> REPLACE\n```" + `

2. To create a new file or completely overwrite an existing file:
` + "```write:path/to/file.ext\ncomplete file contents\n```" + `

Be direct, precise, and concise. Ensure SEARCH blocks match the target file character-for-character.`

// ServeHTTP handles the chat completions endpoint with transparent fallback failovers.
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

	// 6. Build provider request with Coding Agent System Prompt
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

	systemDirective := codingAgentSystemPrompt
	if cavemanEnabled {
		systemDirective += "\nRespond directly and concisely. No fluff, no filler, no pleasantries. Optimize for brevity and token savings."
	}

	hasSystem := false
	for _, m := range reqBody.Messages {
		if strings.ToLower(m.Role) == "system" {
			provReq.Messages = append(provReq.Messages, provider.Message{
				Role:    m.Role,
				Content: m.Content + "\n" + systemDirective,
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
		provReq.Messages = append([]provider.Message{{Role: "system", Content: systemDirective}}, provReq.Messages...)
	}

	// 7. Resolve routing policy
	policy, err := h.policyStore.GetActiveByProject(ctx, tc.Project.ID)
	if err != nil {
		log.Warn("no active routing policy, using dynamic multi-tier fallback plan", zap.Error(err))
	}

	// 8. Build Route Plan with Candidate Chain
	plan, err := h.routingEngine.RoutePlan(provReq, policy)
	if err != nil {
		writeError(w, requestID, err)
		return
	}

	// 9. Execute with failover chain
	start := time.Now()
	var lastErr error
	var successfulResp *provider.ChatResponse
	var successfulCandidate *routing.Candidate

	for i, cand := range plan.Candidates {
		provReq.Model = cand.Model
		log.Info("attempting model execution",
			zap.String("model", cand.Model),
			zap.String("provider", cand.ProviderID),
			zap.Int("candidate_index", i),
		)

		resp, err := cand.Provider.Chat(ctx, provReq)
		if err == nil && resp != nil {
			successfulResp = resp
			successfulCandidate = &cand
			plan.SelectedModel = cand.Model
			plan.ProviderID = cand.ProviderID
			if i > 0 {
				plan.RedirectSummary = fmt.Sprintf("Redirected to %s (%s) after %d failed attempt(s)", cand.Model, cand.ProviderID, i)
			}
			break
		}

		// Record failed attempt in fallback trace
		errMsg := err.Error()
		log.Warn("candidate execution failed, triggering failover",
			zap.String("failed_model", cand.Model),
			zap.String("provider", cand.ProviderID),
			zap.Error(err),
		)

		plan.FallbackTrace = append(plan.FallbackTrace, routing.FallbackAttempt{
			Model:      cand.Model,
			ProviderID: cand.ProviderID,
			Tier:       cand.Tier,
			Error:      errMsg,
			Reason:     "Provider call returned error, redirecting to next available candidate in pool",
		})
		lastErr = err
		h.metrics.ProviderFallbackTotal.WithLabelValues(cand.ProviderID, "fallback").Inc()
	}

	latencyMS := time.Since(start).Milliseconds()

	// If all candidates failed
	if successfulResp == nil {
		record := domain.NewRequestRecord(requestID, tc.Project.ID)
		record.Fail(domain.CodeOf(lastErr), latencyMS)
		h.saveAndEmit(ctx, record, tc.Project.ID)
		writeError(w, requestID, lastErr)
		return
	}

	// 10. Record metrics and complete audit record
	record := domain.NewRequestRecord(requestID, tc.Project.ID)
	if policy != nil {
		record.RoutingStrategy = policy.Strategy
	}
	cost := estimateCost(successfulCandidate.Model, successfulResp.Usage)
	record.Complete(domain.ProviderID(successfulCandidate.ProviderID), successfulCandidate.Model, domain.TokenUsage{
		InputTokens:  successfulResp.Usage.InputTokens,
		OutputTokens: successfulResp.Usage.OutputTokens,
		TotalTokens:  successfulResp.Usage.TotalTokens,
	}, latencyMS, cost)

	if len(plan.FallbackTrace) > 0 {
		record.FallbackUsed = true
		record.RetryCount = len(plan.FallbackTrace)
	}

	h.metrics.ProviderRequestsTotal.WithLabelValues(successfulCandidate.ProviderID, successfulCandidate.Model, "success").Inc()
	h.metrics.ProviderRequestDuration.WithLabelValues(successfulCandidate.ProviderID, successfulCandidate.Model).Observe(float64(latencyMS) / 1000)
	h.metrics.LLMTokensTotal.WithLabelValues("input", successfulCandidate.ProviderID, successfulCandidate.Model).Add(float64(successfulResp.Usage.InputTokens))
	h.metrics.LLMTokensTotal.WithLabelValues("output", successfulCandidate.ProviderID, successfulCandidate.Model).Add(float64(successfulResp.Usage.OutputTokens))

	// 11. Set response headers
	w.Header().Set("X-Nexus-Route-Model", plan.SelectedModel)
	w.Header().Set("X-Nexus-Route-Provider", plan.ProviderID)
	w.Header().Set("X-Nexus-Route-Tier", plan.Tier)
	w.Header().Set("X-Nexus-Route-Complexity", fmt.Sprintf("%.2f", plan.Complexity))
	w.Header().Set("X-Nexus-Route-Intent", plan.Intent)
	w.Header().Set("X-Nexus-Route-Rationale", plan.Rationale)
	if len(plan.FallbackTrace) > 0 {
		w.Header().Set("X-Nexus-Fallback-Count", fmt.Sprintf("%d", len(plan.FallbackTrace)))
		w.Header().Set("X-Nexus-Redirect-Summary", plan.RedirectSummary)
	}

	// 12. Save record asynchronously
	h.saveAndEmit(ctx, record, tc.Project.ID)

	// 13. Write response
	writeJSON(w, http.StatusOK, toOpenAIResponse(successfulResp, requestID, plan))
}

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
		routingMap := map[string]interface{}{
			"requested_model":  decision.RequestedModel,
			"selected_model":   decision.SelectedModel,
			"provider":         decision.ProviderID,
			"tier":             decision.Tier,
			"complexity_score": decision.Complexity,
			"intent":           decision.Intent,
			"rationale":        decision.Rationale,
			"context_chars":    decision.ContextChars,
		}
		if len(decision.FallbackTrace) > 0 {
			routingMap["fallback_trace"] = decision.FallbackTrace
			routingMap["redirect_summary"] = decision.RedirectSummary
		}
		res["nexus_routing"] = routingMap
	}

	return res
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