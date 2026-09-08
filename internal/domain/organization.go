package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// OrgStatus represents the lifecycle state of an organization.
type OrgStatus string

const (
	OrgStatusActive    OrgStatus = "active"
	OrgStatusSuspended OrgStatus = "suspended"
	OrgStatusDeleted   OrgStatus = "deleted"
)

// Organization is the top-level tenant boundary.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    OrgStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewOrganization instantiates a new Organization with an org_ prefixed UUID.
func NewOrganization(name string) *Organization {
	now := time.Now().UTC()
	return &Organization{
		ID:        "org_" + uuid.New().String(),
		Name:      strings.TrimSpace(name),
		Status:    OrgStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Validate checks organization invariants.
func (o *Organization) Validate() error {
	if strings.TrimSpace(o.Name) == "" {
		return New(CodeBadRequest, "organization name is required")
	}
	if o.Status != OrgStatusActive && o.Status != OrgStatusSuspended && o.Status != OrgStatusDeleted {
		return New(CodeBadRequest, "invalid organization status")
	}
	return nil
}
