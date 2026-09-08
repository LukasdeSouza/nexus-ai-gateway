// Package tenant provides tenant resolution and multi-tenant isolation contexts.
package tenant

import (
	"context"
	"fmt"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

type tenantContextKey struct{}

// ProjectStore abstracts project persistence.
type ProjectStore interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
}

// OrganizationStore abstracts organization persistence.
type OrganizationStore interface {
	GetByID(ctx context.Context, id string) (*domain.Organization, error)
}

// TenantContext wraps the resolved tenant hierarchy for an authenticated request.
type TenantContext struct {
	Organization *domain.Organization `json:"organization"`
	Project      *domain.Project      `json:"project"`
	APIKey       *domain.APIKey       `json:"api_key"`
}

// Resolver resolves the tenant context from an API key.
type Resolver struct {
	projects ProjectStore
	orgs     OrganizationStore
}

// NewResolver initializes a tenant Resolver.
func NewResolver(projects ProjectStore, orgs OrganizationStore) *Resolver {
	return &Resolver{
		projects: projects,
		orgs:     orgs,
	}
}

// Resolve validates project and organization hierarchy and status.
func (r *Resolver) Resolve(ctx context.Context, key *domain.APIKey) (*TenantContext, error) {
	if key == nil {
		return nil, domain.ErrUnauthorized
	}

	project, err := r.projects.GetByID(ctx, key.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project %s: %w", key.ProjectID, err)
	}
	if project == nil || project.Status != domain.ProjectStatusActive {
		return nil, domain.New(domain.CodeForbidden, "project is inactive or suspended")
	}

	org, err := r.orgs.GetByID(ctx, project.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve organization %s: %w", project.OrganizationID, err)
	}
	if org == nil || org.Status != domain.OrgStatusActive {
		return nil, domain.New(domain.CodeForbidden, "organization is inactive or suspended")
	}

	return &TenantContext{
		Organization: org,
		Project:      project,
		APIKey:       key,
	}, nil
}

// WithTenant stores the resolved TenantContext in the request context.
func WithTenant(ctx context.Context, tc *TenantContext) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tc)
}

// TenantFromContext retrieves the TenantContext from the request context.
func TenantFromContext(ctx context.Context) (*TenantContext, bool) {
	if ctx == nil {
		return nil, false
	}
	tc, ok := ctx.Value(tenantContextKey{}).(*TenantContext)
	return tc, ok
}
