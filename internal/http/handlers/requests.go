package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// RequestInspector abstracts request record lookup.
type RequestInspector interface {
	GetByRequestID(ctx context.Context, requestID string) (*domain.RequestRecord, error)
}

// NewRequestsHandler returns an http.HandlerFunc for GET /v1/requests/{requestID}.
func NewRequestsHandler(inspector RequestInspector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		reqID := observability.RequestIDFromContext(ctx)
		targetID := chi.URLParam(r, "requestID")

		rec, err := inspector.GetByRequestID(ctx, targetID)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusOK, rec)
	}
}
