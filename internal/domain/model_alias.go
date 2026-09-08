package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Well-known model aliases.
const (
	ModelSmart   = "smart"
	ModelCheap   = "cheap"
	ModelFast    = "fast"
	ModelQuality = "quality"
)

// Capabilities describes the features of a model.
type Capabilities struct {
	Chat                 bool    `json:"chat"`
	Streaming            bool    `json:"streaming"`
	FunctionCall         bool    `json:"function_call"`
	Vision               bool    `json:"vision"`
	MaxContextTokens     int     `json:"max_context_tokens"`
	InputPricePerMToken  float64 `json:"input_price_per_mtoken"`
	OutputPricePerMToken float64 `json:"output_price_per_mtoken"`
}

// ModelAlias maps a virtual alias (e.g., "smart") or standard name to a provider's model.
type ModelAlias struct {
	ID             string       `json:"id"`
	Alias          string       `json:"alias"`
	Provider       ProviderID   `json:"provider"`
	ProviderModel  string       `json:"provider_model"`
	Capabilities   Capabilities `json:"capabilities"`
	Active         bool         `json:"active"`
	CreatedAt      time.Time    `json:"created_at"`
}

// DefaultModelAliases returns the baseline default model aliases and pricing.
func DefaultModelAliases() []ModelAlias {
	now := time.Now().UTC()
	return []ModelAlias{
		{
			ID:            "ma_" + uuid.New().String(),
			Alias:         ModelSmart,
			Provider:      ProviderOpenAI,
			ProviderModel: "gpt-4o",
			Capabilities: Capabilities{
				Chat:                 true,
				Streaming:            true,
				FunctionCall:         true,
				Vision:               true,
				MaxContextTokens:     128000,
				InputPricePerMToken:  2.50,
				OutputPricePerMToken: 10.00,
			},
			Active:    true,
			CreatedAt: now,
		},
		{
			ID:            "ma_" + uuid.New().String(),
			Alias:         ModelCheap,
			Provider:      ProviderOpenAI,
			ProviderModel: "gpt-4o-mini",
			Capabilities: Capabilities{
				Chat:                 true,
				Streaming:            true,
				FunctionCall:         true,
				Vision:               true,
				MaxContextTokens:     128000,
				InputPricePerMToken:  0.15,
				OutputPricePerMToken: 0.60,
			},
			Active:    true,
			CreatedAt: now,
		},
		{
			ID:            "ma_" + uuid.New().String(),
			Alias:         ModelFast,
			Provider:      ProviderGemini,
			ProviderModel: "gemini-1.5-flash",
			Capabilities: Capabilities{
				Chat:                 true,
				Streaming:            true,
				FunctionCall:         true,
				Vision:               true,
				MaxContextTokens:     1000000,
				InputPricePerMToken:  0.075,
				OutputPricePerMToken: 0.30,
			},
			Active:    true,
			CreatedAt: now,
		},
		{
			ID:            "ma_" + uuid.New().String(),
			Alias:         ModelQuality,
			Provider:      ProviderAnthropic,
			ProviderModel: "claude-3-5-sonnet-20241022",
			Capabilities: Capabilities{
				Chat:                 true,
				Streaming:            true,
				FunctionCall:         true,
				Vision:               true,
				MaxContextTokens:     200000,
				InputPricePerMToken:  3.00,
				OutputPricePerMToken: 15.00,
			},
			Active:    true,
			CreatedAt: now,
		},
	}
}

// Validate checks model alias fields.
func (m *ModelAlias) Validate() error {
	if strings.TrimSpace(m.Alias) == "" {
		return New(CodeBadRequest, "alias is required")
	}
	if strings.TrimSpace(m.ProviderModel) == "" {
		return New(CodeBadRequest, "provider_model is required")
	}
	return nil
}
