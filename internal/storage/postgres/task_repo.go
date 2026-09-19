package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// TaskRepo manages persistence of RemoteTask entities.
type TaskRepo struct {
	pool *pgxpool.Pool
}

// NewTaskRepo returns a new TaskRepo.
func NewTaskRepo(pool *pgxpool.Pool) *TaskRepo {
	return &TaskRepo{pool: pool}
}

// Create inserts a new remote task record.
func (r *TaskRepo) Create(ctx context.Context, t *domain.RemoteTask) error {
	query := `
		INSERT INTO tasks (
			id, project_id, prompt, preset, policy, budget, status,
			result_summary, diff, claimed_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.pool.Exec(ctx, query,
		t.ID,
		t.ProjectID,
		t.Prompt,
		t.Preset,
		t.Policy,
		t.Budget,
		string(t.Status),
		t.ResultSummary,
		t.Diff,
		t.ClaimedBy,
		t.CreatedAt,
		t.UpdatedAt,
	)
	return err
}

// GetByID retrieves a task by ID.
func (r *TaskRepo) GetByID(ctx context.Context, id string) (*domain.RemoteTask, error) {
	query := `
		SELECT id, project_id, prompt, preset, policy, budget, status,
		       result_summary, diff, claimed_by, created_at, updated_at
		FROM tasks
		WHERE id = $1
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, id))
}

// ListPending returns pending tasks for a project.
func (r *TaskRepo) ListPending(ctx context.Context, projectID string) ([]*domain.RemoteTask, error) {
	query := `
		SELECT id, project_id, prompt, preset, policy, budget, status,
		       result_summary, diff, claimed_by, created_at, updated_at
		FROM tasks
		WHERE project_id = $1 AND status = 'pending'
		ORDER BY created_at ASC
		LIMIT 10
	`
	rows, err := r.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.RemoteTask
	for rows.Next() {
		t, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// Claim transitions a pending task to running status for workerID.
func (r *TaskRepo) Claim(ctx context.Context, id, workerID string) (*domain.RemoteTask, error) {
	query := `
		UPDATE tasks
		SET status = 'running', claimed_by = $1, updated_at = $2
		WHERE id = $3 AND status = 'pending'
		RETURNING id, project_id, prompt, preset, policy, budget, status,
		          result_summary, diff, claimed_by, created_at, updated_at
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, workerID, time.Now().UTC(), id))
}

// Complete updates task execution results (completed or failed).
func (r *TaskRepo) Complete(ctx context.Context, id, summary, diff string, status domain.TaskStatus) error {
	query := `
		UPDATE tasks
		SET status = $1, result_summary = $2, diff = $3, updated_at = $4
		WHERE id = $5
	`
	tag, err := r.pool.Exec(ctx, query, string(status), summary, diff, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *TaskRepo) scanRow(row pgx.Row) (*domain.RemoteTask, error) {
	var t domain.RemoteTask
	var status string
	var summary, diff, claimedBy *string
	err := row.Scan(
		&t.ID,
		&t.ProjectID,
		&t.Prompt,
		&t.Preset,
		&t.Policy,
		&t.Budget,
		&status,
		&summary,
		&diff,
		&claimedBy,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	t.Status = domain.TaskStatus(status)
	if summary != nil {
		t.ResultSummary = *summary
	}
	if diff != nil {
		t.Diff = *diff
	}
	if claimedBy != nil {
		t.ClaimedBy = *claimedBy
	}
	return &t, nil
}
