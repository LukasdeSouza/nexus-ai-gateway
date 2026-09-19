package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

type mockTaskStore struct {
	tasks map[string]*domain.RemoteTask
}

func (m *mockTaskStore) Create(ctx context.Context, task *domain.RemoteTask) error {
	m.tasks[task.ID] = task
	return nil
}

func (m *mockTaskStore) GetByID(ctx context.Context, id string) (*domain.RemoteTask, error) {
	if t, ok := m.tasks[id]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockTaskStore) ListPending(ctx context.Context, projectID string) ([]*domain.RemoteTask, error) {
	var res []*domain.RemoteTask
	for _, t := range m.tasks {
		if t.ProjectID == projectID && t.Status == domain.TaskStatusPending {
			res = append(res, t)
		}
	}
	return res, nil
}

func (m *mockTaskStore) Claim(ctx context.Context, id, workerID string) (*domain.RemoteTask, error) {
	if t, ok := m.tasks[id]; ok {
		if t.Status != domain.TaskStatusPending {
			return nil, domain.New(domain.CodeConflict, "task not pending")
		}
		t.Status = domain.TaskStatusRunning
		t.ClaimedBy = workerID
		return t, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockTaskStore) Complete(ctx context.Context, id, summary, diff string, status domain.TaskStatus) error {
	if t, ok := m.tasks[id]; ok {
		t.Status = status
		t.ResultSummary = summary
		t.Diff = diff
		return nil
	}
	return domain.ErrNotFound
}

func TestTasksHandler(t *testing.T) {
	store := &mockTaskStore{tasks: make(map[string]*domain.RemoteTask)}
	router := NewTasksRouter(store)

	t.Run("POST /v1/tasks creates remote task", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"project_id": "prj_test123",
			"prompt":     "Refactor logging in handlers",
			"preset":     "build",
			"policy":     "safe-auto",
			"budget":     0.50,
		})

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var created domain.RemoteTask
		err := json.NewDecoder(rec.Body).Decode(&created)
		require.NoError(t, err)

		assert.Equal(t, "prj_test123", created.ProjectID)
		assert.Equal(t, "Refactor logging in handlers", created.Prompt)
		assert.Equal(t, domain.TaskStatusPending, created.Status)

		// Test GET pending
		reqPending := httptest.NewRequest(http.MethodGet, "/pending?project_id=prj_test123", nil)
		recPending := httptest.NewRecorder()
		router.ServeHTTP(recPending, reqPending)
		assert.Equal(t, http.StatusOK, recPending.Code)

		// Test Claim
		reqClaim := httptest.NewRequest(http.MethodPost, "/"+created.ID+"/claim", bytes.NewReader([]byte(`{"worker_id":"worker-1"}`)))
		recClaim := httptest.NewRecorder()
		router.ServeHTTP(recClaim, reqClaim)
		assert.Equal(t, http.StatusOK, recClaim.Code)

		// Test Complete
		reqComplete := httptest.NewRequest(http.MethodPost, "/"+created.ID+"/complete", bytes.NewReader([]byte(`{"summary":"Refactored logger","diff":"+ logger.Info()","failed":false}`)))
		recComplete := httptest.NewRecorder()
		router.ServeHTTP(recComplete, reqComplete)
		assert.Equal(t, http.StatusOK, recComplete.Code)

		// Verify task status
		task, err := store.GetByID(context.Background(), created.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusCompleted, task.Status)
		assert.Equal(t, "Refactored logger", task.ResultSummary)
	})
}
