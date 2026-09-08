package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// ProviderRepo manages persistence of ProviderConnection records.
type ProviderRepo struct {
	pool *pgxpool.Pool
}

// NewProviderRepo returns a new ProviderRepo.
func NewProviderRepo(pool *pgxpool.Pool) *ProviderRepo {
	return &ProviderRepo{pool: pool}
}

// Create inserts a new provider connection.
func (r *ProviderRepo) Create(ctx context.Context, p *domain.ProviderConnection) error {
	query := `
		INSERT INTO provider_connections (id, project_id, provider, secret_ref, enabled, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, provider) DO UPDATE
		SET secret_ref = EXCLUDED.secret_ref, enabled = EXCLUDED.enabled
	`
	_, err := r.pool.Exec(ctx, query, p.ID, p.ProjectID, string(p.Provider), p.SecretRef, p.Enabled, p.CreatedAt)
	return err
}

// GetByID retrieves a connection by ID.
func (r *ProviderRepo) GetByID(ctx context.Context, id string) (*domain.ProviderConnection, error) {
	query := `
		SELECT id, project_id, provider, secret_ref, enabled, created_at
		FROM provider_connections
		WHERE id = $1
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, id))
}

// GetByProjectAndProvider retrieves a provider connection for a specific project and provider.
func (r *ProviderRepo) GetByProjectAndProvider(ctx context.Context, projectID string, p domain.ProviderID) (*domain.ProviderConnection, error) {
	query := `
		SELECT id, project_id, provider, secret_ref, enabled, created_at
		FROM provider_connections
		WHERE project_id = $1 AND provider = $2
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, projectID, string(p)))
}

// ListByProject returns all provider connections configured for a project.
func (r *ProviderRepo) ListByProject(ctx context.Context, projectID string) ([]*domain.ProviderConnection, error) {
	query := `
		SELECT id, project_id, provider, secret_ref, enabled, created_at
		FROM provider_connections
		WHERE project_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []*domain.ProviderConnection
	for rows.Next() {
		var conn domain.ProviderConnection
		var prov string
		if err := rows.Scan(&conn.ID, &conn.ProjectID, &prov, &conn.SecretRef, &conn.Enabled, &conn.CreatedAt); err != nil {
			return nil, err
		}
		conn.Provider = domain.ProviderID(prov)
		conns = append(conns, &conn)
	}
	return conns, rows.Err()
}

// Delete removes a provider connection.
func (r *ProviderRepo) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM provider_connections WHERE id = $1`
	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ProviderRepo) scanRow(row pgx.Row) (*domain.ProviderConnection, error) {
	var conn domain.ProviderConnection
	var prov string
	err := row.Scan(
		&conn.ID,
		&conn.ProjectID,
		&prov,
		&conn.SecretRef,
		&conn.Enabled,
		&conn.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	conn.Provider = domain.ProviderID(prov)
	return &conn, nil
}
