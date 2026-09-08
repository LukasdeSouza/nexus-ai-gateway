package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// ModelAliasStore abstracts model aliases lookup.
type ModelAliasStore interface {
	ListActive(ctx context.Context) ([]*domain.ModelAlias, error)
}

// ModelsHandler returns an http.HandlerFunc for GET /v1/models.
func NewModelsHandler(store ModelAliasStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		reqID := observability.RequestIDFromContext(ctx)

		aliases, err := store.ListActive(ctx)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		type modelItem struct {
			ID           string              `json:"id"`
			Object       string              `json:"object"`
			Created      int64               `json:"created"`
			OwnedBy      string              `json:"owned_by"`
			Capabilities domain.Capabilities `json:"capabilities"`
		}

		data := make([]modelItem, 0, len(aliases))
		for _, a := range aliases {
			data = append(data, modelItem{
				ID:           a.Alias,
				Object:       "model",
				Created:      a.CreatedAt.Unix(),
				OwnedBy:      string(a.Provider),
				Capabilities: a.Capabilities,
			})
		}

		// Always include well-known routing aliases
		for _, wellKnown := range []string{"smart", "cheap", "fast", "quality"} {
			hasIt := false
			for _, d := range data {
				if d.ID == wellKnown {
					hasIt = true
					break
				}
			}
			if !hasIt {
				data = append(data, modelItem{
					ID:      wellKnown,
					Object:  "model",
					Created: time.Now().Unix(),
					OwnedBy: "nexus",
					Capabilities: domain.Capabilities{
						Chat:      true,
						Streaming: true,
					},
				})
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"object": "list",
			"data":   data,
		})
	}
}
