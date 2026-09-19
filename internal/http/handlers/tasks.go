package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// TaskStore abstracts persistence of RemoteTask entities.
type TaskStore interface {
	Create(ctx context.Context, task *domain.RemoteTask) error
	GetByID(ctx context.Context, id string) (*domain.RemoteTask, error)
	ListPending(ctx context.Context, projectID string) ([]*domain.RemoteTask, error)
	Claim(ctx context.Context, id, workerID string) (*domain.RemoteTask, error)
	Complete(ctx context.Context, id, summary, diff string, status domain.TaskStatus) error
}

// NewTasksRouter creates a sub-router for /tasks endpoints.
func NewTasksRouter(store TaskStore) http.Handler {
	r := chi.NewRouter()

	// POST /v1/tasks - Enqueue a new remote task (from Web UI)
	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		var body struct {
			ProjectID string  `json:"project_id"`
			Prompt    string  `json:"prompt"`
			Preset    string  `json:"preset"`
			Policy    string  `json:"policy"`
			Budget    float64 `json:"budget"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, reqID, err)
			return
		}

		task := domain.NewRemoteTask(body.ProjectID, body.Prompt, body.Preset, body.Policy, body.Budget)
		if err := task.Validate(); err != nil {
			writeError(w, reqID, err)
			return
		}

		if err := store.Create(r.Context(), task); err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusCreated, task)
	})

	// GET /v1/tasks/pending - Fetch pending tasks for a project (used by CLI worker)
	r.Get("/pending", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		projectID := r.URL.Query().Get("project_id")
		if projectID == "" {
			writeError(w, reqID, domain.New(domain.CodeBadRequest, "project_id parameter is required"))
			return
		}

		tasks, err := store.ListPending(r.Context(), projectID)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tasks": tasks,
		})
	})

	// GET /v1/tasks/{id} - Get task details & live diff preview
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		id := chi.URLParam(r, "id")

		task, err := store.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusOK, task)
	})

	// POST /v1/tasks/{id}/claim - Worker claims task
	r.Post("/{id}/claim", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		id := chi.URLParam(r, "id")
		var body struct {
			WorkerID string `json:"worker_id"`
		}
		_ = decodeJSON(r, &body)
		if body.WorkerID == "" {
			body.WorkerID = "switchyard-cli-worker"
		}

		task, err := store.Claim(r.Context(), id, body.WorkerID)
		if err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusOK, task)
	})

	// POST /v1/tasks/{id}/complete - Worker completes task
	r.Post("/{id}/complete", func(w http.ResponseWriter, r *http.Request) {
		reqID := observability.RequestIDFromContext(r.Context())
		id := chi.URLParam(r, "id")
		var body struct {
			Summary string `json:"summary"`
			Diff    string `json:"diff"`
			Failed  bool   `json:"failed"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, reqID, err)
			return
		}

		status := domain.TaskStatusCompleted
		if body.Failed {
			status = domain.TaskStatusFailed
		}

		if err := store.Complete(r.Context(), id, body.Summary, body.Diff, status); err != nil {
			writeError(w, reqID, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": string(status),
		})
	})

	return r
}
