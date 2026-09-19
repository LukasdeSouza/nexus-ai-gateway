package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/auth"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// APIKeyStore abstracts API key persistence.
type APIKeyStore interface {
	Create(ctx context.Context, key *domain.APIKey) error
	Revoke(ctx context.Context, id string) error
	ListByProject(ctx context.Context, projectID string) ([]*domain.APIKey, error)
	GetByHash(ctx context.Context, hash string) (*domain.APIKey, error)
}

// NewAPIKeysRouter creates a sub-router for /api-keys endpoints.
func NewAPIKeysRouter(store APIKeyStore) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		var body struct {
			ProjectID string         `json:"project_id"`
			Env       string         `json:"env"`
			Scopes    []domain.Scope `json:"scopes"`
			ExpiresIn *int           `json:"expires_in_seconds,omitempty"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, reqID, err)
			return
		}

		plaintext, prefix, err := auth.GenerateKey(body.Env)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		hash, err := auth.HashKey(plaintext)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		var expiresAt *time.Time
		if body.ExpiresIn != nil && *body.ExpiresIn > 0 {
			exp := time.Now().UTC().Add(time.Duration(*body.ExpiresIn) * time.Second)
			expiresAt = &exp
		}

		key := domain.NewAPIKey(body.ProjectID, hash, prefix, body.Scopes, expiresAt)
		if err := key.Validate(); err != nil {
			writeError(w, reqID, err)
			return
		}

		if err := store.Create(r.Context(), key); err != nil {
			writeError(w, reqID, err)
			return
		}

		type createResponse struct {
			*domain.APIKey
			Key string `json:"key"`
		}

		writeJSON(w, http.StatusCreated, createResponse{
			APIKey: key,
			Key:    plaintext,
		})
	})

	r.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		id := chi.URLParam(r, "id")

		if err := store.Revoke(r.Context(), id); err != nil {
			writeError(w, reqID, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	return r
}
