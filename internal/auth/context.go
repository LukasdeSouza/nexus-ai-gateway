package auth

import (
	"context"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

type apiKeyContextKey struct{}
type projectIDContextKey struct{}
type orgIDContextKey struct{}

// WithAPIKey stores the authenticated APIKey in context.
func WithAPIKey(ctx context.Context, key *domain.APIKey) context.Context {
	return context.WithValue(ctx, apiKeyContextKey{}, key)
}

// APIKeyFromContext retrieves the APIKey from context.
func APIKeyFromContext(ctx context.Context) (*domain.APIKey, bool) {
	if ctx == nil {
		return nil, false
	}
	key, ok := ctx.Value(apiKeyContextKey{}).(*domain.APIKey)
	return key, ok
}

// WithProjectID stores the authenticated project ID in context.
func WithProjectID(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, projectIDContextKey{}, projectID)
}

// ProjectIDFromContext retrieves the project ID from context.
func ProjectIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	pID, ok := ctx.Value(projectIDContextKey{}).(string)
	return pID, ok
}

// WithOrgID stores the authenticated organization ID in context.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDContextKey{}, orgID)
}

// OrgIDFromContext retrieves the organization ID from context.
func OrgIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	oID, ok := ctx.Value(orgIDContextKey{}).(string)
	return oID, ok
}
