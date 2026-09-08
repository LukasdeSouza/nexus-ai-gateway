package domain

import (
	"time"

	"github.com/google/uuid"
)

// UsageEvent is an immutable event published to the asynchronous event pipeline.
type UsageEvent struct {
	EventID          string        `json:"event_id"`
	RequestID        string        `json:"request_id"`
	ProjectID        string        `json:"project_id"`
	Provider         ProviderID    `json:"provider"`
	Model            string        `json:"model"`
	RoutingStrategy  Strategy      `json:"routing_strategy"`
	LatencyMS        int64         `json:"latency_ms"`
	InputTokens      int64         `json:"input_tokens"`
	OutputTokens     int64         `json:"output_tokens"`
	TotalTokens      int64         `json:"total_tokens"`
	EstimatedCostUSD float64       `json:"estimated_cost_usd"`
	FallbackUsed     bool          `json:"fallback_used"`
	RetryCount       int           `json:"retry_count"`
	Status           RequestStatus `json:"status"`
	Timestamp        time.Time     `json:"timestamp"`
}

// NewUsageEvent constructs an immutable event from a RequestRecord.
func NewUsageEvent(record *RequestRecord) *UsageEvent {
	return &UsageEvent{
		EventID:          "evt_" + uuid.New().String(),
		RequestID:        record.RequestID,
		ProjectID:        record.ProjectID,
		Provider:         record.Provider,
		Model:            record.Model,
		RoutingStrategy:  record.RoutingStrategy,
		LatencyMS:        record.LatencyMS,
		InputTokens:      record.InputTokens,
		OutputTokens:     record.OutputTokens,
		TotalTokens:      record.TotalTokens,
		EstimatedCostUSD: record.EstimatedCostUSD,
		FallbackUsed:     record.FallbackUsed,
		RetryCount:       record.RetryCount,
		Status:           record.Status,
		Timestamp:        time.Now().UTC(),
	}
}
