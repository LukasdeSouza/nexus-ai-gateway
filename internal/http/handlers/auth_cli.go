// Package handlers contains all HTTP request handlers for the gateway.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/auth"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// CliAuthHandlerConfig holds dependencies for the CLI authentication endpoint.
type CliAuthHandlerConfig struct {
	OrgStore     OrganizationStore
	ProjectStore ProjectStore
	KeyStore     APIKeyStore
	SupabaseURL  string
	SupabaseKey  string
	Logger       *zap.Logger
}

// CliAuthHandler handles POST /v1/auth/cli-key.
// It accepts a Supabase JWT from the web frontend, validates it against Supabase Auth,
// provisions an Organization and Project for the user if they don't already exist,
// generates a new CLI API Key, and returns the credentials.
type CliAuthHandler struct {
	orgStore     OrganizationStore
	projectStore ProjectStore
	keyStore     APIKeyStore
	supabaseURL  string
	supabaseKey  string
	httpClient   *http.Client
	logger       *zap.Logger
}

// NewCliAuthHandler creates a new CliAuthHandler.
func NewCliAuthHandler(cfg CliAuthHandlerConfig) *CliAuthHandler {
	return &CliAuthHandler{
		orgStore:     cfg.OrgStore,
		projectStore: cfg.ProjectStore,
		keyStore:     cfg.KeyStore,
		supabaseURL:  strings.TrimRight(cfg.SupabaseURL, "/"),
		supabaseKey:  cfg.SupabaseKey,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		logger:       cfg.Logger,
	}
}

type supabaseUserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (h *CliAuthHandler) validateSupabaseToken(ctx context.Context, token string) (*supabaseUserResponse, error) {
	url := fmt.Sprintf("%s/auth/v1/user", h.supabaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create supabase auth request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	if h.supabaseKey != "" {
		req.Header.Set("apikey", h.supabaseKey)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contact supabase auth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("supabase auth rejected token (status %d): %s", resp.StatusCode, string(body))
	}

	var user supabaseUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode supabase user response: %w", err)
	}

	if user.ID == "" {
		return nil, fmt.Errorf("supabase returned empty user ID")
	}

	return &user, nil
}

// ServeHTTP implements http.Handler for POST /v1/auth/cli-key.
func (h *CliAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqID := observability.RequestIDFromContext(r.Context())

	if r.Method != http.MethodPost {
		writeError(w, reqID, domain.New(domain.CodeBadRequest, "method not allowed"))
		return
	}

	// 1. Extract token from Authorization header or JSON payload
	var token string
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}

	if token == "" {
		var reqBody struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
			token = reqBody.AccessToken
		}
	}

	token = strings.TrimSpace(token)
	if token == "" {
		writeError(w, reqID, domain.New(domain.CodeUnauthorized, "missing authentication token"))
		return
	}

	// 2. Validate token against Supabase Auth
	sbUser, err := h.validateSupabaseToken(r.Context(), token)
	if err != nil {
		h.logger.Warn("supabase token validation failed", zap.Error(err), zap.String("request_id", reqID))
		writeError(w, reqID, domain.New(domain.CodeUnauthorized, "invalid or expired authentication token"))
		return
	}

	// 3. Ensure Organization exists
	orgID := "org_" + sbUser.ID
	org, err := h.orgStore.GetByID(r.Context(), orgID)
	if err != nil {
		orgName := sbUser.Email
		if orgName == "" {
			orgName = "User-" + sbUser.ID[:min(8, len(sbUser.ID))]
		} else if idx := strings.Index(orgName, "@"); idx > 0 {
			orgName = orgName[:idx] + "'s Org"
		}

		newOrg := &domain.Organization{
			ID:        orgID,
			Name:      orgName,
			Status:    domain.OrgStatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if createErr := h.orgStore.Create(r.Context(), newOrg); createErr != nil {
			// In case created concurrently
			if fetched, fetchErr := h.orgStore.GetByID(r.Context(), orgID); fetchErr == nil {
				org = fetched
			} else {
				h.logger.Error("failed to create organization", zap.Error(createErr))
				writeError(w, reqID, domain.New(domain.CodeInternal, "failed to provision organization"))
				return
			}
		} else {
			org = newOrg
		}
	}

	// 4. Ensure Project exists
	prjID := "prj_" + sbUser.ID
	prj, err := h.projectStore.GetByID(r.Context(), prjID)
	if err != nil {
		newPrj := &domain.Project{
			ID:             prjID,
			OrganizationID: org.ID,
			Name:           "Default Project",
			Environment:    domain.EnvProduction,
			Status:         domain.ProjectStatusActive,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}
		if createErr := h.projectStore.Create(r.Context(), newPrj); createErr != nil {
			if fetched, fetchErr := h.projectStore.GetByID(r.Context(), prjID); fetchErr == nil {
				prj = fetched
			} else {
				h.logger.Error("failed to create project", zap.Error(createErr))
				writeError(w, reqID, domain.New(domain.CodeInternal, "failed to provision project"))
				return
			}
		} else {
			prj = newPrj
		}
	}

	// 5. Generate and persist new API key for the project
	plaintext, prefix, err := auth.GenerateKey("live")
	if err != nil {
		h.logger.Error("failed to generate api key", zap.Error(err))
		writeError(w, reqID, domain.New(domain.CodeInternal, "failed to generate api key"))
		return
	}

	hash, err := auth.HashKey(plaintext)
	if err != nil {
		h.logger.Error("failed to hash api key", zap.Error(err))
		writeError(w, reqID, domain.New(domain.CodeInternal, "failed to hash api key"))
		return
	}

	key := domain.NewAPIKey(prj.ID, hash, prefix, []domain.Scope{
		domain.ScopeAdmin,
		domain.ScopeInferenceWrite,
		domain.ScopeUsageRead,
	}, nil)

	if err := h.keyStore.Create(r.Context(), key); err != nil {
		h.logger.Error("failed to persist api key", zap.Error(err))
		writeError(w, reqID, domain.New(domain.CodeInternal, "failed to store api key"))
		return
	}

	h.logger.Info("provisioned cli api key for user",
		zap.String("user_id", sbUser.ID),
		zap.String("email", sbUser.Email),
		zap.String("project_id", prj.ID),
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"api_key":         plaintext,
		"project_id":      prj.ID,
		"organization_id": org.ID,
		"email":           sbUser.Email,
	})
}

// RotateKey handles POST /v1/auth/rotate-key to securely rotate an existing CLI key.
func (h *CliAuthHandler) RotateKey(w http.ResponseWriter, r *http.Request) {
	reqID := observability.RequestIDFromContext(r.Context())

	var token string
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		writeError(w, reqID, domain.New(domain.CodeUnauthorized, "missing API key to rotate"))
		return
	}

	hash, err := auth.HashKey(token)
	if err != nil {
		writeError(w, reqID, domain.New(domain.CodeBadRequest, "invalid API key format"))
		return
	}

	oldKey, err := h.keyStore.GetByHash(r.Context(), hash)
	if err != nil || !oldKey.IsActive() {
		writeError(w, reqID, domain.New(domain.CodeUnauthorized, "key is invalid, expired, or already revoked"))
		return
	}

	// Revoke old key
	_ = h.keyStore.Revoke(r.Context(), oldKey.ID)

	// Generate new key
	plaintext, prefix, err := auth.GenerateKey("live")
	if err != nil {
		h.logger.Error("failed to generate new rotated key", zap.Error(err))
		writeError(w, reqID, domain.New(domain.CodeInternal, "failed to rotate api key"))
		return
	}

	newHash, err := auth.HashKey(plaintext)
	if err != nil {
		h.logger.Error("failed to hash new rotated key", zap.Error(err))
		writeError(w, reqID, domain.New(domain.CodeInternal, "failed to rotate api key"))
		return
	}

	newKey := domain.NewAPIKey(oldKey.ProjectID, newHash, prefix, oldKey.Scopes, nil)
	if err := h.keyStore.Create(r.Context(), newKey); err != nil {
		h.logger.Error("failed to persist rotated api key", zap.Error(err))
		writeError(w, reqID, domain.New(domain.CodeInternal, "failed to store rotated api key"))
		return
	}

	h.logger.Info("rotated cli api key successfully",
		zap.String("project_id", oldKey.ProjectID),
		zap.String("old_key_id", oldKey.ID),
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"api_key":    plaintext,
		"project_id": oldKey.ProjectID,
	})
}
