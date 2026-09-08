package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Environment represents deployment environment.
type Environment string

const (
	EnvProduction  Environment = "production"
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
)

// ProjectStatus represents the state of a project.
type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusSuspended ProjectStatus = "suspended"
	ProjectStatusDeleted   ProjectStatus = "deleted"
)

// Project represents an isolated project within an organization.
type Project struct {
	ID             string        `json:"id"`
	OrganizationID string        `json:"organization_id"`
	Name           string        `json:"name"`
	Environment    Environment   `json:"environment"`
	Status         ProjectStatus `json:"status"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// NewProject creates a new Project instance.
func NewProject(orgID, name string, env Environment) *Project {
	now := time.Now().UTC()
	if env == "" {
		env = EnvProduction
	}
	return &Project{
		ID:             "prj_" + uuid.New().String(),
		OrganizationID: orgID,
		Name:           strings.TrimSpace(name),
		Environment:    env,
		Status:         ProjectStatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// Validate checks project invariants.
func (p *Project) Validate() error {
	if strings.TrimSpace(p.OrganizationID) == "" {
		return New(CodeBadRequest, "organization_id is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return New(CodeBadRequest, "project name is required")
	}
	if p.Environment != EnvProduction && p.Environment != EnvDevelopment && p.Environment != EnvStaging {
		return New(CodeBadRequest, "invalid environment")
	}
	return nil
}
