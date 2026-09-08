package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// ProviderStore abstracts provider connections persistence.
type ProviderStore interface {
	Create(ctx context.Context, p *domain.ProviderConnection) error
	ListByProject(ctx context.Context, projectID string) ([]*domain.ProviderConnection, error)
	Delete(ctx context.Context, id string) error
}

// NewProvidersRouter creates a sub-router for /providers endpoints.
func NewProvidersRouter(store ProviderStore) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		var body struct {
			ProjectID string            `json:"project_id"`
			Provider  domain.ProviderID `json:"provider"`
			SecretRef string            `json:"secret_ref"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, reqID, err)
			return
		}

		conn := domain.NewProviderConnection(body.ProjectID, body.Provider, body.SecretRef)
		if err := conn.Validate(); err != nil {
			writeError(w, reqID, err)
			return
		}

		if err := store.Create(r.Context(), conn); err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusCreated, conn)
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		projectID := r.URL.Query().Get("project_id")
		if projectID == "" {
			writeError(w, reqID, domain.New(domain.CodeBadRequest, "project_id query param is required"))
			return
		}

		conns, err := store.ListByProject(r.Context(), projectID)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": conns,
		})
	})

	return r
}
