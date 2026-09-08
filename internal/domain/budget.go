package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// BudgetPeriod represents the evaluation cycle.
type BudgetPeriod string

const (
	BudgetPeriodDaily   BudgetPeriod = "daily"
	BudgetPeriodWeekly  BudgetPeriod = "weekly"
	BudgetPeriodMonthly BudgetPeriod = "monthly"
)

// BudgetLimitType defines the metric tracked against the budget.
type BudgetLimitType string

const (
	BudgetLimitUSD      BudgetLimitType = "usd"
	BudgetLimitTokens   BudgetLimitType = "tokens"
	BudgetLimitRequests BudgetLimitType = "requests"
)

// BudgetAction is the enforcement action when limit is reached.
type BudgetAction string

const (
	BudgetActionReject BudgetAction = "reject"
	BudgetActionAlert  BudgetAction = "alert"
)

// Budget represents a project-level spend or volume quota.
type Budget struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id"`
	Period    BudgetPeriod    `json:"period"`
	LimitType BudgetLimitType `json:"limit_type"`
	Limit     float64         `json:"limit"`
	Action    BudgetAction    `json:"action"`
	CreatedAt time.Time       `json:"created_at"`
}

// NewBudget creates a new Budget instance.
func NewBudget(projectID string, period BudgetPeriod, limitType BudgetLimitType, limit float64, action BudgetAction) *Budget {
	if action == "" {
		action = BudgetActionReject
	}
	return &Budget{
		ID:        "bgt_" + uuid.New().String(),
		ProjectID: projectID,
		Period:    period,
		LimitType: limitType,
		Limit:     limit,
		Action:    action,
		CreatedAt: time.Now().UTC(),
	}
}

// Validate checks budget invariants.
func (b *Budget) Validate() error {
	if strings.TrimSpace(b.ProjectID) == "" {
		return New(CodeBadRequest, "project_id is required")
	}
	if b.Limit <= 0 {
		return New(CodeBadRequest, "limit must be greater than zero")
	}
	return nil
}
