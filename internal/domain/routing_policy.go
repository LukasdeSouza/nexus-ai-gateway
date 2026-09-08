package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Strategy represents the candidate selection algorithm.
type Strategy string

const (
	StrategyExplicit Strategy = "explicit"
	StrategyCheapest Strategy = "cheapest"
	StrategyFastest  Strategy = "fastest"
	StrategyBalanced Strategy = "balanced"
	StrategyQuality  Strategy = "quality"
)

// Candidate represents a candidate provider/model option in a routing policy.
type Candidate struct {
	Provider ProviderID `json:"provider"`
	Model    string     `json:"model"`
	Weight   float64    `json:"weight,omitempty"`
	Priority int        `json:"priority,omitempty"`
}

// FallbackPolicy dictates how the gateway reacts to provider failures.
type FallbackPolicy struct {
	Enabled          bool  `json:"enabled"`
	MaxFallbacks     int   `json:"max_fallbacks"`
	EligibleOnStatus []int `json:"eligible_on_status,omitempty"`
}

// RoutingPolicy defines routing rules and candidate selection for a project.
type RoutingPolicy struct {
	ID             string          `json:"id"`
	ProjectID      string          `json:"project_id"`
	Name           string          `json:"name"`
	Strategy       Strategy        `json:"strategy"`
	Candidates     []Candidate     `json:"candidates"`
	FallbackPolicy *FallbackPolicy `json:"fallback_policy,omitempty"`
	Active         bool            `json:"active"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// NewRoutingPolicy creates a new RoutingPolicy instance.
func NewRoutingPolicy(projectID, name string, strategy Strategy, candidates []Candidate) *RoutingPolicy {
	now := time.Now().UTC()
	if strategy == "" {
		strategy = StrategyBalanced
	}
	return &RoutingPolicy{
		ID:         "pol_" + uuid.New().String(),
		ProjectID:  projectID,
		Name:       strings.TrimSpace(name),
		Strategy:   strategy,
		Candidates: candidates,
		FallbackPolicy: &FallbackPolicy{
			Enabled:          true,
			MaxFallbacks:     2,
			EligibleOnStatus: []int{429, 500, 502, 503, 504},
		},
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Validate checks routing policy fields.
func (p *RoutingPolicy) Validate() error {
	if strings.TrimSpace(p.ProjectID) == "" {
		return New(CodeBadRequest, "project_id is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return New(CodeBadRequest, "policy name is required")
	}
	if len(p.Candidates) == 0 {
		return New(CodeBadRequest, "at least one candidate is required")
	}
	for _, c := range p.Candidates {
		if strings.TrimSpace(c.Model) == "" {
			return New(CodeBadRequest, "candidate model is required")
		}
	}
	return nil
}
