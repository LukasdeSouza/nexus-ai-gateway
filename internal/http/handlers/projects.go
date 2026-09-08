package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// ProjectStore abstracts project persistence for the handler.
type ProjectStore interface {
	Create(ctx context.Context, p *domain.Project) error
	GetByID(ctx context.Context, id string) (*domain.Project, error)
	ListByOrganization(ctx context.Context, orgID string) ([]*domain.Project, error)
}

// NewProjectsRouter creates a sub-router for /projects endpoints.
func NewProjectsRouter(store ProjectStore) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		var body struct {
			OrganizationID string             `json:"organization_id"`
			Name           string             `json:"name"`
			Environment    domain.Environment `json:"environment"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, reqID, err)
			return
		}

		p := domain.NewProject(body.OrganizationID, body.Name, body.Environment)
		if err := p.Validate(); err != nil {
			writeError(w, reqID, err)
			return
		}

		if err := store.Create(r.Context(), p); err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusCreated, p)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		id := chi.URLParam(r, "id")

		p, err := store.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusOK, p)
	})

	return r
}
