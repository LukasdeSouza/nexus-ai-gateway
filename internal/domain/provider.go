package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProviderID identifies an upstream model provider.
type ProviderID string

const (
	ProviderOpenAI    ProviderID = "openai"
	ProviderAnthropic ProviderID = "anthropic"
	ProviderGemini    ProviderID = "gemini"
)

// ProviderConnection stores BYOK credentials reference for a project.
type ProviderConnection struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Provider  ProviderID `json:"provider"`
	SecretRef string     `json:"secret_ref"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"created_at"`
}

// NewProviderConnection creates a new BYOK provider connection.
func NewProviderConnection(projectID string, provider ProviderID, secretRef string) *ProviderConnection {
	return &ProviderConnection{
		ID:        "pconn_" + uuid.New().String(),
		ProjectID: projectID,
		Provider:  provider,
		SecretRef: secretRef,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
	}
}

// Validate checks provider connection invariants.
func (p *ProviderConnection) Validate() error {
	if strings.TrimSpace(p.ProjectID) == "" {
		return New(CodeBadRequest, "project_id is required")
	}
	switch p.Provider {
	case ProviderOpenAI, ProviderAnthropic, ProviderGemini:
	default:
		return New(CodeBadRequest, "unsupported provider: "+string(p.Provider))
	}
	if strings.TrimSpace(p.SecretRef) == "" {
		return New(CodeBadRequest, "secret_ref is required")
	}
	return nil
}
