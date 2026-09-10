// Package routing provides semantic and heuristic routing intelligence for Nexus AI Gateway.
package routing

import (
	"fmt"
	"strings"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
)

// Candidate represents a candidate model to try in sequence.
type Candidate struct {
	Provider   provider.Provider `json:"-"`
	ProviderID string            `json:"provider_id"`
	Model      string            `json:"model"`
	Tier       string            `json:"tier"` // "primary" | "same-tier-fallback" | "cross-tier-fallback"
}

// FallbackAttempt records an attempt and why it failed before redirecting.
type FallbackAttempt struct {
	Model      string `json:"model"`
	ProviderID string `json:"provider_id"`
	Tier       string `json:"tier"`
	Error      string `json:"error"`
	Reason     string `json:"reason"`
}

// Decision holds the complete routing metadata, thought process, and any fallback trace.
type Decision struct {
	RequestedModel  string            `json:"requested_model"`
	SelectedModel   string            `json:"selected_model"`
	ProviderID      string            `json:"provider_id"`
	Tier            string            `json:"tier"`
	Complexity      float64           `json:"complexity_score"`
	Intent          string            `json:"intent"`
	Rationale       string            `json:"rationale"`
	ContextChars    int               `json:"context_chars"`
	Candidates      []Candidate       `json:"-"`
	FallbackTrace   []FallbackAttempt `json:"fallback_trace,omitempty"`
	RedirectSummary string            `json:"redirect_summary,omitempty"`
}

// Engine evaluates chat messages and builds prioritized candidate chains for resilient execution.
type Engine struct {
	providerReg *provider.Registry
}

// NewEngine creates a new routing engine instance.
func NewEngine(reg *provider.Registry) *Engine {
	return &Engine{
		providerReg: reg,
	}
}

// RoutePlan builds a prioritized candidate chain and initial decision for the request.
func (e *Engine) RoutePlan(req *provider.ChatRequest, policy *domain.RoutingPolicy) (*Decision, error) {
	all := e.providerReg.All()
	if len(all) == 0 {
		return nil, domain.New(domain.CodeProviderUnavailable, "no providers registered")
	}

	// Active providers
	geminiProv, hasGemini := e.providerReg.Get("gemini")
	openaiProv, hasOpenAI := e.providerReg.Get("openai")
	anthropicProv, hasAnthropic := e.providerReg.Get("anthropic")

	// Custom project routing policy if defined
	if policy != nil && len(policy.Candidates) > 0 {
		var candidates []Candidate
		for i, c := range policy.Candidates {
			if p, ok := e.providerReg.Get(string(c.Provider)); ok {
				tier := "primary"
				if i > 0 {
					tier = "policy-fallback"
				}
				candidates = append(candidates, Candidate{
					Provider:   p,
					ProviderID: string(c.Provider),
					Model:      c.Model,
					Tier:       tier,
				})
			}
		}
		if len(candidates) > 0 {
			return &Decision{
				RequestedModel: req.Model,
				SelectedModel:  candidates[0].Model,
				ProviderID:     candidates[0].ProviderID,
				Tier:           "custom-policy",
				Complexity:     0.5,
				Intent:         "Custom Project Policy",
				Rationale:      fmt.Sprintf("Policy defined with %d failover candidates", len(candidates)),
				ContextChars:   countChars(req.Messages),
				Candidates:     candidates,
			}, nil
		}
	}

	model := req.Model
	if model == "" {
		model = "auto"
	}

	// Model pools
	var fastPool []Candidate
	if hasGemini {
		fastPool = append(fastPool, Candidate{Provider: geminiProv, ProviderID: "gemini", Model: "gemini-3.6-flash", Tier: "fast"})
	}
	if hasOpenAI {
		fastPool = append(fastPool, Candidate{Provider: openaiProv, ProviderID: "openai", Model: "gpt-4o-mini", Tier: "fast"})
	}
	if hasAnthropic {
		fastPool = append(fastPool, Candidate{Provider: anthropicProv, ProviderID: "anthropic", Model: "claude-3-haiku-20240307", Tier: "fast"})
	}

	var smartPool []Candidate
	if hasOpenAI {
		smartPool = append(smartPool, Candidate{Provider: openaiProv, ProviderID: "openai", Model: "gpt-4o", Tier: "smart"})
	}
	if hasAnthropic {
		smartPool = append(smartPool, Candidate{Provider: anthropicProv, ProviderID: "anthropic", Model: "claude-3-5-sonnet-20241022", Tier: "smart"})
	}
	if hasGemini {
		smartPool = append(smartPool, Candidate{Provider: geminiProv, ProviderID: "gemini", Model: "gemini-2.5-pro", Tier: "smart"})
	}

	ctxChars := countChars(req.Messages)
	var candidates []Candidate
	var initialTier string
	var complexity float64
	var intent string
	var rationale string

	switch {
	case model == "auto":
		complexity, intent, rationale = analyzeComplexity(req.Messages)
		if complexity >= 0.5 {
			initialTier = "smart"
			// Start with smart candidates, failover to remaining smart, then cross-tier to fast
			candidates = append(candidates, smartPool...)
			candidates = append(candidates, fastPool...)
		} else {
			initialTier = "fast"
			// Start with fast candidates, failover to remaining fast, then cross-tier to smart
			candidates = append(candidates, fastPool...)
			candidates = append(candidates, smartPool...)
		}

	case model == "cheap" || model == "fast":
		initialTier = "fast"
		complexity = 0.2
		intent = "Explicit Tier Selection"
		rationale = "User requested fast/cheap tier execution"
		candidates = append(candidates, fastPool...)
		candidates = append(candidates, smartPool...) // cross-tier safety net

	case model == "smart" || model == "quality":
		initialTier = "smart"
		complexity = 0.8
		intent = "Explicit Tier Selection"
		rationale = "User requested smart/quality tier execution"
		candidates = append(candidates, smartPool...)
		candidates = append(candidates, fastPool...) // cross-tier safety net

	case strings.HasPrefix(model, "claude-"):
		initialTier = "explicit-model"
		complexity = 0.7
		intent = "Specific Model Pinning"
		rationale = "User pinned Anthropic Claude model family"
		if hasAnthropic {
			candidates = append(candidates, Candidate{Provider: anthropicProv, ProviderID: "anthropic", Model: model, Tier: "primary"})
		}
		// Fallbacks
		candidates = append(candidates, smartPool...)
		candidates = append(candidates, fastPool...)

	case strings.HasPrefix(model, "gemini-"):
		initialTier = "explicit-model"
		complexity = 0.3
		intent = "Specific Model Pinning"
		rationale = "User pinned Google Gemini model family"
		if hasGemini {
			candidates = append(candidates, Candidate{Provider: geminiProv, ProviderID: "gemini", Model: model, Tier: "primary"})
		}
		// Fallbacks
		candidates = append(candidates, fastPool...)
		candidates = append(candidates, smartPool...)

	case strings.HasPrefix(model, "gpt-"):
		initialTier = "explicit-model"
		complexity = 0.7
		intent = "Specific Model Pinning"
		rationale = "User pinned OpenAI GPT model family"
		if hasOpenAI {
			candidates = append(candidates, Candidate{Provider: openaiProv, ProviderID: "openai", Model: model, Tier: "primary"})
		}
		// Fallbacks
		candidates = append(candidates, smartPool...)
		candidates = append(candidates, fastPool...)

	default:
		if p, ok := e.providerReg.Get(model); ok {
			candidates = append(candidates, Candidate{Provider: p, ProviderID: model, Model: model, Tier: "primary"})
		}
		candidates = append(candidates, fastPool...)
		candidates = append(candidates, smartPool...)
		initialTier = "fallback-fast"
		complexity = 0.2
		intent = "Custom / Fallback Routing"
		rationale = "Routing through registered models"
	}

	// Deduplicate candidates
	candidates = deduplicateCandidates(candidates)

	if len(candidates) == 0 {
		candidates = append(candidates, Candidate{
			Provider:   all[0],
			ProviderID: all[0].ID(),
			Model:      "gemini-3.6-flash",
			Tier:       "fallback",
		})
	}

	return &Decision{
		RequestedModel: model,
		SelectedModel:  candidates[0].Model,
		ProviderID:     candidates[0].ProviderID,
		Tier:           initialTier,
		Complexity:     complexity,
		Intent:         intent,
		Rationale:      rationale,
		ContextChars:   ctxChars,
		Candidates:     candidates,
	}, nil
}

func deduplicateCandidates(list []Candidate) []Candidate {
	seen := make(map[string]bool)
	var res []Candidate
	for _, c := range list {
		key := c.ProviderID + ":" + c.Model
		if !seen[key] {
			seen[key] = true
			res = append(res, c)
		}
	}
	return res
}

func countChars(messages []provider.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	return total
}

func analyzeComplexity(messages []provider.Message) (score float64, intent string, rationale string) {
	if len(messages) == 0 {
		return 0.1, "Empty Query", "No input detected"
	}

	var lastUserContent string
	totalLen := 0
	for _, m := range messages {
		totalLen += len(m.Content)
		if strings.ToLower(m.Role) == "user" {
			lastUserContent = m.Content
		}
	}

	lower := strings.ToLower(lastUserContent)

	heavyKeywords := map[string]string{
		"refactor":                "Code Refactoring & Architecture",
		"architecture":            "System Architecture Design",
		"arquitetura":             "System Architecture Design",
		"concurrency":             "Concurrent Systems Analysis",
		"concorrência":            "Concurrent Systems Analysis",
		"deadlock":                "Deadlock & Race Condition Analysis",
		"race condition":          "Deadlock & Race Condition Analysis",
		"benchmark":               "Performance Benchmarking",
		"distributed transaction": "Distributed Systems Coordination",
		"transação distribuída":   "Distributed Systems Coordination",
		"microservices":           "Microservices Architecture",
		"kubernetes":              "Infrastructure & Orchestration",
		"security audit":          "Security Vulnerability Audit",
		"complexidade de tempo":   "Algorithm Complexity Proof",
		"big-o":                   "Algorithm Complexity Proof",
		"deep analysis":           "In-depth Technical Analysis",
		"análise aprofundada":     "In-depth Technical Analysis",
	}

	for kw, intnt := range heavyKeywords {
		if strings.Contains(lower, kw) {
			return 0.85, intnt, fmt.Sprintf("High technical complexity pattern matched: '%s'", kw)
		}
	}

	if strings.Contains(lastUserContent, "```") || strings.Contains(lastUserContent, "func ") || strings.Contains(lastUserContent, "class ") || strings.Contains(lastUserContent, "struct ") {
		return 0.75, "Source Code Analysis", "Embedded source code or structured syntax block detected"
	}

	if totalLen > 3500 {
		return 0.70, "Large Context Analysis", fmt.Sprintf("Long conversation history (%d characters)", totalLen)
	}

	conversationalKeywords := []string{
		"hi", "hello", "hey", "olá", "ola", "bom dia", "boa tarde", "boa noite",
		"ok", "yes", "no", "sim", "não", "nao", "thanks", "obrigado", "valeu",
	}

	for _, kw := range conversationalKeywords {
		if lower == kw || strings.HasPrefix(lower, kw+" ") || strings.HasSuffix(lower, " "+kw) {
			return 0.05, "Conversational / Greeting", "Short conversational interaction"
		}
	}

	if len(lastUserContent) < 120 {
		return 0.15, "Direct Inquiry", "Short factual or instructional prompt"
	}

	return 0.35, "Standard Query", "Moderate length query without heavy technical keywords"
}