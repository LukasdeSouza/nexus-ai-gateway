package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// UsageInspector abstracts querying past request records for usage totals.
type UsageInspector interface {
	ListByProject(ctx context.Context, projectID string, limit, offset int) ([]*domain.RequestRecord, error)
}

// NewUsageHandler returns an http.HandlerFunc for GET /v1/usage.
func NewUsageHandler(inspector UsageInspector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		reqID := observability.RequestIDFromContext(ctx)

		projectID := r.URL.Query().Get("project_id")
		if projectID == "" {
			writeError(w, reqID, domain.New(domain.CodeBadRequest, "project_id query param is required"))
			return
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		records, err := inspector.ListByProject(ctx, projectID, limit, offset)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		var totalInput, totalOutput int64
		var totalCost float64
		for _, rec := range records {
			totalInput += rec.InputTokens
			totalOutput += rec.OutputTokens
			totalCost += rec.EstimatedCostUSD
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"project_id":           projectID,
			"total_requests":       len(records),
			"total_input_tokens":   totalInput,
			"total_output_tokens":  totalOutput,
			"total_tokens":         totalInput + totalOutput,
			"total_estimated_cost": totalCost,
			"records":              records,
		})
	}
}
