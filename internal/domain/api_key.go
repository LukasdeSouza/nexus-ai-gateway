package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// KeyStatus represents API key state.
type KeyStatus string

const (
	KeyStatusActive  KeyStatus = "active"
	KeyStatusRevoked KeyStatus = "revoked"
	KeyStatusExpired KeyStatus = "expired"
)

// Scope represents a fine-grained permission scope.
type Scope string

const (
	ScopeInferenceWrite Scope = "inference:write"
	ScopeUsageRead      Scope = "usage:read"
	ScopeProjectsRead   Scope = "projects:read"
	ScopeProjectsWrite  Scope = "projects:write"
	ScopeRoutingRead    Scope = "routing:read"
	ScopeRoutingWrite   Scope = "routing:write"
	ScopeKeysWrite      Scope = "keys:write"
	ScopeAlertsWrite    Scope = "alerts:write"
	ScopeAdmin          Scope = "admin"
)

// APIKey is a gateway authentication credential.
type APIKey struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Hash      string     `json:"-"`
	Prefix    string     `json:"prefix"`
	Status    KeyStatus  `json:"status"`
	Scopes    []Scope    `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// NewAPIKey creates a domain APIKey instance with default scopes.
func NewAPIKey(projectID, hash, prefix string, scopes []Scope, expiresAt *time.Time) *APIKey {
	if len(scopes) == 0 {
		scopes = []Scope{ScopeInferenceWrite, ScopeUsageRead}
	}
	return &APIKey{
		ID:        "key_" + uuid.New().String(),
		ProjectID: projectID,
		Hash:      hash,
		Prefix:    prefix,
		Status:    KeyStatusActive,
		Scopes:    scopes,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
}

// IsExpired checks if the key has passed its expiration time.
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*k.ExpiresAt)
}

// IsActive checks if the key is in active status and not expired.
func (k *APIKey) IsActive() bool {
	if k.Status != KeyStatusActive {
		return false
	}
	return !k.IsExpired()
}

// HasScope checks whether the key has the given scope or admin scope.
func (k *APIKey) HasScope(s Scope) bool {
	for _, sc := range k.Scopes {
		if sc == ScopeAdmin || sc == s {
			return true
		}
	}
	return false
}

// Validate checks API key fields.
func (k *APIKey) Validate() error {
	if strings.TrimSpace(k.ProjectID) == "" {
		return New(CodeBadRequest, "project_id is required")
	}
	if strings.TrimSpace(k.Hash) == "" {
		return New(CodeBadRequest, "key hash is required")
	}
	if strings.TrimSpace(k.Prefix) == "" {
		return New(CodeBadRequest, "key prefix is required")
	}
	return nil
}
