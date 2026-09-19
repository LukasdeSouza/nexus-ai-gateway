package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/auth"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

type mockOrgStore struct {
	orgs map[string]*domain.Organization
}

func (m *mockOrgStore) Create(ctx context.Context, org *domain.Organization) error {
	m.orgs[org.ID] = org
	return nil
}

func (m *mockOrgStore) GetByID(ctx context.Context, id string) (*domain.Organization, error) {
	if o, ok := m.orgs[id]; ok {
		return o, nil
	}
	return nil, domain.ErrNotFound
}

type mockProjectStore struct {
	projects map[string]*domain.Project
}

func (m *mockProjectStore) Create(ctx context.Context, p *domain.Project) error {
	m.projects[p.ID] = p
	return nil
}

func (m *mockProjectStore) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	if p, ok := m.projects[id]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockProjectStore) ListByOrganization(ctx context.Context, orgID string) ([]*domain.Project, error) {
	var res []*domain.Project
	for _, p := range m.projects {
		if p.OrganizationID == orgID {
			res = append(res, p)
		}
	}
	return res, nil
}

type mockKeyStore struct {
	keys map[string]*domain.APIKey
}

func (m *mockKeyStore) Create(ctx context.Context, key *domain.APIKey) error {
	m.keys[key.ID] = key
	return nil
}

func (m *mockKeyStore) Revoke(ctx context.Context, id string) error {
	if k, ok := m.keys[id]; ok {
		k.Status = domain.KeyStatusRevoked
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockKeyStore) ListByProject(ctx context.Context, projectID string) ([]*domain.APIKey, error) {
	var res []*domain.APIKey
	for _, k := range m.keys {
		if k.ProjectID == projectID {
			res = append(res, k)
		}
	}
	return res, nil
}

func (m *mockKeyStore) GetByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	for _, k := range m.keys {
		if k.Hash == hash {
			return k, nil
		}
	}
	return nil, domain.ErrNotFound
}

func TestCliAuthHandler_ServeHTTP(t *testing.T) {
	// Mock Supabase Auth Server
	mockSupabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/v1/user", r.URL.Path)
		auth := r.Header.Get("Authorization")
		if auth == "Bearer valid-test-token" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":    "user-uuid-1234",
				"email": "developer@switchyard.dev",
			})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "invalid token"})
	}))
	defer mockSupabase.Close()

	orgStore := &mockOrgStore{orgs: make(map[string]*domain.Organization)}
	prjStore := &mockProjectStore{projects: make(map[string]*domain.Project)}
	keyStore := &mockKeyStore{keys: make(map[string]*domain.APIKey)}

	handler := NewCliAuthHandler(CliAuthHandlerConfig{
		OrgStore:     orgStore,
		ProjectStore: prjStore,
		KeyStore:     keyStore,
		SupabaseURL:  mockSupabase.URL,
		SupabaseKey:  "test-anon-key",
		Logger:       zap.NewNop(),
	})

	t.Run("rejects missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/cli-key", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("rejects invalid supabase token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/cli-key", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("authorizes valid token and provisions credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/cli-key", nil)
		req.Header.Set("Authorization", "Bearer valid-test-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)

		apiKey, ok := resp["api_key"].(string)
		require.True(t, ok)
		assert.True(t, len(apiKey) > 10)
		assert.Contains(t, apiKey, "ngk_live_")

		projectID, ok := resp["project_id"].(string)
		require.True(t, ok)
		assert.Equal(t, "prj_user-uuid-1234", projectID)

		assert.Equal(t, "developer@switchyard.dev", resp["email"])

		// Verify DB provisioning
		org, err := orgStore.GetByID(context.Background(), "org_user-uuid-1234")
		require.NoError(t, err)
		assert.Equal(t, "developer's Org", org.Name)

		prj, err := prjStore.GetByID(context.Background(), "prj_user-uuid-1234")
		require.NoError(t, err)
		assert.Equal(t, "Default Project", prj.Name)

		assert.NotEmpty(t, keyStore.keys)
	})
}

func TestCliAuthHandler_RotateKey(t *testing.T) {
	keyStore := &mockKeyStore{keys: make(map[string]*domain.APIKey)}
	handler := NewCliAuthHandler(CliAuthHandlerConfig{
		KeyStore: keyStore,
		Logger:   zap.NewNop(),
	})

	// Pre-seed an active key
	plaintext, prefix, err := auth.GenerateKey("live")
	require.NoError(t, err)
	hash, err := auth.HashKey(plaintext)
	require.NoError(t, err)

	existingKey := domain.NewAPIKey("prj_123", hash, prefix, []domain.Scope{domain.ScopeAdmin}, nil)
	keyStore.keys[existingKey.ID] = existingKey

	t.Run("rotates valid key successfully", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/rotate-key", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()

		handler.RotateKey(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)

		newKey, ok := resp["api_key"].(string)
		require.True(t, ok)
		assert.NotEqual(t, plaintext, newKey)
		assert.Equal(t, "prj_123", resp["project_id"])

		// Verify old key was revoked
		assert.Equal(t, domain.KeyStatusRevoked, existingKey.Status)
	})

	t.Run("rejects already revoked key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/rotate-key", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()

		handler.RotateKey(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}
