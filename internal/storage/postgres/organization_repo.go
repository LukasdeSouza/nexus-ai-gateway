package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// OrganizationRepo manages persistence of Organization entities.
type OrganizationRepo struct {
	pool *pgxpool.Pool
}

// NewOrganizationRepo returns a new OrganizationRepo.
func NewOrganizationRepo(pool *pgxpool.Pool) *OrganizationRepo {
	return &OrganizationRepo{pool: pool}
}

// Create inserts a new organization.
func (r *OrganizationRepo) Create(ctx context.Context, org *domain.Organization) error {
	query := `
		INSERT INTO organizations (id, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.pool.Exec(ctx, query, org.ID, org.Name, string(org.Status), org.CreatedAt, org.UpdatedAt)
	return err
}

// GetByID retrieves an organization by ID.
func (r *OrganizationRepo) GetByID(ctx context.Context, id string) (*domain.Organization, error) {
	query := `
		SELECT id, name, status, created_at, updated_at
		FROM organizations
		WHERE id = $1
	`
	var org domain.Organization
	var status string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&org.ID,
		&org.Name,
		&status,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	org.Status = domain.OrgStatus(status)
	return &org, nil
}

// Update updates an organization's name and status.
func (r *OrganizationRepo) Update(ctx context.Context, org *domain.Organization) error {
	org.UpdatedAt = time.Now().UTC()
	query := `
		UPDATE organizations
		SET name = $1, status = $2, updated_at = $3
		WHERE id = $4
	`
	tag, err := r.pool.Exec(ctx, query, org.Name, string(org.Status), org.UpdatedAt, org.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete marks an organization as deleted (soft delete).
func (r *OrganizationRepo) Delete(ctx context.Context, id string) error {
	query := `
		UPDATE organizations
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
