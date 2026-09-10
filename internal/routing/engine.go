// Package routing provides semantic and heuristic routing intelligence for Nexus AI Gateway.
package routing

import (
	"fmt"
	"strings"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
)

// Decision holds the complete routing metadata and thought process.
type Decision struct {
	RequestedModel string  `json:"requested_model"`
	SelectedModel  string  `json:"selected_model"`
	ProviderID     string  `json:"provider_id"`
	Tier           string  `json:"tier"` // "fast" | "smart"
	Complexity     float64 `json:"complexity_score"` // 0.0 to 1.0
	Intent         string  `json:"intent"`
	Rationale      string  `json:"rationale"`
	ContextChars   int     `json:"context_chars"`
}

// Engine evaluates chat messages and selects optimal models and providers.
type Engine struct {
	providerReg *provider.Registry
}

// NewEngine creates a new routing engine instance.
func NewEngine(reg *provider.Registry) *Engine {
	return &Engine{
		providerReg: reg,
	}
}

// Route determines the best provider and model for a request, returning full decision metadata.
func (e *Engine) Route(req *provider.ChatRequest, policy *domain.RoutingPolicy) (provider.Provider, *Decision, error) {
	all := e.providerReg.All()
	if len(all) == 0 {
		return nil, nil, domain.New(domain.CodeProviderUnavailable, "no providers registered")
	}

	if policy != nil && len(policy.Candidates) > 0 {
		c := policy.Candidates[0]
		if p, ok := e.providerReg.Get(string(c.Provider)); ok {
			d := &Decision{
				RequestedModel: req.Model,
				SelectedModel:  c.Model,
				ProviderID:     string(c.Provider),
				Tier:           "custom-policy",
				Complexity:     0.5,
				Intent:         "Custom Project Policy",
				Rationale:      fmt.Sprintf("Explicit project policy matched: candidate priority %d", c.Priority),
				ContextChars:   countChars(req.Messages),
			}
			return p, d, nil
		}
	}

	model := req.Model
	if model == "" {
		model = "auto"
	}

	_, hasGemini := e.providerReg.Get("gemini")
	_, hasOpenAI := e.providerReg.Get("openai")
	_, hasAnthropic := e.providerReg.Get("anthropic")

	pickFastTier := func() (provider.Provider, string, string) {
		if hasGemini {
			p, _ := e.providerReg.Get("gemini")
			return p, "gemini-3.6-flash", "gemini"
		}
		if hasOpenAI {
			p, _ := e.providerReg.Get("openai")
			return p, "gpt-4o-mini", "openai"
		}
		if hasAnthropic {
			p, _ := e.providerReg.Get("anthropic")
			return p, "claude-3-haiku-20240307", "anthropic"
		}
		return all[0], "gemini-3.6-flash", all[0].ID()
	}

	pickSmartTier := func() (provider.Provider, string, string) {
		if hasOpenAI {
			p, _ := e.providerReg.Get("openai")
			return p, "gpt-4o", "openai"
		}
		if hasAnthropic {
			p, _ := e.providerReg.Get("anthropic")
			return p, "claude-3-5-sonnet-20241022", "anthropic"
		}
		if hasGemini {
			p, _ := e.providerReg.Get("gemini")
			return p, "gemini-2.5-pro", "gemini"
		}
		return all[0], "gpt-4o", all[0].ID()
	}

	var prov provider.Provider
	var chosenModel string
	var chosenProviderID string
	var tier string
	var complexity float64
	var intent string
	var rationale string

	ctxChars := countChars(req.Messages)

	switch {
	case model == "auto":
		complexity, intent, rationale = analyzeComplexity(req.Messages)
		if complexity >= 0.5 {
			tier = "smart"
			prov, chosenModel, chosenProviderID = pickSmartTier()
		} else {
			tier = "fast"
			prov, chosenModel, chosenProviderID = pickFastTier()
		}

	case model == "cheap" || model == "fast":
		tier = "fast"
		complexity = 0.2
		intent = "Explicit Tier Selection"
		rationale = "User requested fast/cheap tier execution"
		prov, chosenModel, chosenProviderID = pickFastTier()

	case model == "smart" || model == "quality":
		tier = "smart"
		complexity = 0.8
		intent = "Explicit Tier Selection"
		rationale = "User requested smart/quality tier execution"
		prov, chosenModel, chosenProviderID = pickSmartTier()

	case strings.HasPrefix(model, "claude-"):
		tier = "explicit-model"
		complexity = 0.7
		intent = "Specific Model Pinning"
		rationale = "User pinned Anthropic Claude model family"
		prov, _ = e.providerReg.Get("anthropic")
		chosenModel = model
		chosenProviderID = "anthropic"

	case strings.HasPrefix(model, "gemini-"):
		tier = "explicit-model"
		complexity = 0.3
		intent = "Specific Model Pinning"
		rationale = "User pinned Google Gemini model family"
		prov, _ = e.providerReg.Get("gemini")
		chosenModel = model
		chosenProviderID = "gemini"

	case strings.HasPrefix(model, "gpt-"):
		tier = "explicit-model"
		complexity = 0.7
		intent = "Specific Model Pinning"
		rationale = "User pinned OpenAI GPT model family"
		prov, _ = e.providerReg.Get("openai")
		chosenModel = model
		chosenProviderID = "openai"

	default:
		if p, ok := e.providerReg.Get(model); ok {
			prov = p
			chosenModel = model
			chosenProviderID = model
			tier = "explicit-provider"
			complexity = 0.5
			intent = "Explicit Provider Selection"
			rationale = "Provider selected by direct identifier"
		} else {
			tier = "fallback-fast"
			complexity = 0.2
			intent = "Fallback Routing"
			rationale = "Unknown model identifier, falling back to active fast tier"
			prov, chosenModel, chosenProviderID = pickFastTier()
		}
	}

	if prov == nil {
		prov = all[0]
		chosenProviderID = all[0].ID()
	}

	decision := &Decision{
		RequestedModel: model,
		SelectedModel:  chosenModel,
		ProviderID:     chosenProviderID,
		Tier:           tier,
		Complexity:     complexity,
		Intent:         intent,
		Rationale:      rationale,
		ContextChars:   ctxChars,
	}

	return prov, decision, nil
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

	// Tier 1 High Complexity matches
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

	// Code block or dense syntax detection
	if strings.Contains(lastUserContent, "```") || strings.Contains(lastUserContent, "func ") || strings.Contains(lastUserContent, "class ") || strings.Contains(lastUserContent, "struct ") {
		return 0.75, "Source Code Analysis", "Embedded source code or structured syntax block detected"
	}

	// Large context
	if totalLen > 3500 {
		return 0.70, "Large Context Analysis", fmt.Sprintf("Long conversation history (%d characters)", totalLen)
	}

	// Conversational / simple tasks
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