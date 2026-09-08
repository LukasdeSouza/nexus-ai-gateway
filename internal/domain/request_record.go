package domain

import (
	"time"

	"github.com/google/uuid"
)

// RequestStatus represents the outcome of an inference request.
type RequestStatus string

const (
	RequestStatusSuccess        RequestStatus = "success"
	RequestStatusFailed         RequestStatus = "failed"
	RequestStatusTimedOut       RequestStatus = "timed_out"
	RequestStatusRateLimited    RequestStatus = "rate_limited"
	RequestStatusBudgetExceeded RequestStatus = "budget_exceeded"
	RequestStatusFallback       RequestStatus = "fallback"
)

// TokenUsage holds token metrics for an LLM interaction.
type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// RequestRecord is the persistent audit record of an inference request.
type RequestRecord struct {
	ID               string        `json:"id"`
	RequestID        string        `json:"request_id"`
	ProjectID        string        `json:"project_id"`
	Provider         ProviderID    `json:"provider"`
	Model            string        `json:"model"`
	RoutingStrategy  Strategy      `json:"routing_strategy"`
	Status           RequestStatus `json:"status"`
	LatencyMS        int64         `json:"latency_ms"`
	InputTokens      int64         `json:"input_tokens"`
	OutputTokens     int64         `json:"output_tokens"`
	TotalTokens      int64         `json:"total_tokens"`
	EstimatedCostUSD float64       `json:"estimated_cost_usd"`
	FallbackUsed     bool          `json:"fallback_used"`
	RetryCount       int           `json:"retry_count"`
	ErrorCode        string        `json:"error_code,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
}

// NewRequestRecord creates an initial pending request record.
func NewRequestRecord(requestID, projectID string) *RequestRecord {
	return &RequestRecord{
		ID:        "rec_" + uuid.New().String(),
		RequestID: requestID,
		ProjectID: projectID,
		Status:    RequestStatusSuccess,
		CreatedAt: time.Now().UTC(),
	}
}

// Complete updates the record with successful completion data.
func (r *RequestRecord) Complete(provider ProviderID, model string, usage TokenUsage, latencyMS int64, cost float64) {
	r.Provider = provider
	r.Model = model
	r.InputTokens = usage.InputTokens
	r.OutputTokens = usage.OutputTokens
	r.TotalTokens = usage.TotalTokens
	r.LatencyMS = latencyMS
	r.EstimatedCostUSD = cost
	r.Status = RequestStatusSuccess
}

// Fail marks the record as failed with the appropriate error code.
func (r *RequestRecord) Fail(code Code, latencyMS int64) {
	r.LatencyMS = latencyMS
	r.ErrorCode = string(code)
	switch code {
	case CodeProviderTimeout:
		r.Status = RequestStatusTimedOut
	case CodeRateLimitExceeded:
		r.Status = RequestStatusRateLimited
	case CodeBudgetExceeded:
		r.Status = RequestStatusBudgetExceeded
	default:
		r.Status = RequestStatusFailed
	}
}
