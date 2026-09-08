package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// ProjectRepo manages persistence of Project entities.
type ProjectRepo struct {
	pool *pgxpool.Pool
}

// NewProjectRepo returns a new ProjectRepo.
func NewProjectRepo(pool *pgxpool.Pool) *ProjectRepo {
	return &ProjectRepo{pool: pool}
}

// Create inserts a new project.
func (r *ProjectRepo) Create(ctx context.Context, p *domain.Project) error {
	query := `
		INSERT INTO projects (id, organization_id, name, environment, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.pool.Exec(ctx, query, p.ID, p.OrganizationID, p.Name, string(p.Environment), string(p.Status), p.CreatedAt, p.UpdatedAt)
	return err
}

// GetByID retrieves a project by ID.
func (r *ProjectRepo) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	query := `
		SELECT id, organization_id, name, environment, status, created_at, updated_at
		FROM projects
		WHERE id = $1
	`
	var p domain.Project
	var env, status string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID,
		&p.OrganizationID,
		&p.Name,
		&env,
		&status,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p.Environment = domain.Environment(env)
	p.Status = domain.ProjectStatus(status)
	return &p, nil
}

// ListByOrganization returns all projects for an organization.
func (r *ProjectRepo) ListByOrganization(ctx context.Context, orgID string) ([]*domain.Project, error) {
	query := `
		SELECT id, organization_id, name, environment, status, created_at, updated_at
		FROM projects
		WHERE organization_id = $1 AND status != 'deleted'
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*domain.Project
	for rows.Next() {
		var p domain.Project
		var env, status string
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &env, &status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Environment = domain.Environment(env)
		p.Status = domain.ProjectStatus(status)
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}

// Update updates a project's name, environment, and status.
func (r *ProjectRepo) Update(ctx context.Context, p *domain.Project) error {
	p.UpdatedAt = time.Now().UTC()
	query := `
		UPDATE projects
		SET name = $1, environment = $2, status = $3, updated_at = $4
		WHERE id = $5
	`
	tag, err := r.pool.Exec(ctx, query, p.Name, string(p.Environment), string(p.Status), p.UpdatedAt, p.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete marks a project as deleted (soft delete).
func (r *ProjectRepo) Delete(ctx context.Context, id string) error {
	query := `
		UPDATE projects
		SET status = 'deleted', updated_at = $1
		WHERE id = $2
	`
	tag, err := r.pool.Exec(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
