package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// OrganizationStore abstracts org persistence for the handler.
type OrganizationStore interface {
	Create(ctx context.Context, org *domain.Organization) error
	GetByID(ctx context.Context, id string) (*domain.Organization, error)
}

// NewOrganizationsRouter creates a sub-router for /organizations endpoints.
func NewOrganizationsRouter(store OrganizationStore) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		var body struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, reqID, err)
			return
		}

		org := domain.NewOrganization(body.Name)
		if err := org.Validate(); err != nil {
			writeError(w, reqID, err)
			return
		}

		if err := store.Create(r.Context(), org); err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusCreated, org)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		id := chi.URLParam(r, "id")

		org, err := store.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusOK, org)
	})

	return r
}
