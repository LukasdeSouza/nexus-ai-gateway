package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// APIKeyRepo manages persistence of APIKey entities.
type APIKeyRepo struct {
	pool *pgxpool.Pool
}

// NewAPIKeyRepo returns a new APIKeyRepo.
func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo {
	return &APIKeyRepo{pool: pool}
}

// Create inserts a new API key record.
func (r *APIKeyRepo) Create(ctx context.Context, key *domain.APIKey) error {
	query := `
		INSERT INTO api_keys (id, project_id, hash, prefix, status, scopes, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	scopesStr := make([]string, len(key.Scopes))
	for i, s := range key.Scopes {
		scopesStr[i] = string(s)
	}

	_, err := r.pool.Exec(ctx, query,
		key.ID,
		key.ProjectID,
		key.Hash,
		key.Prefix,
		string(key.Status),
		scopesStr,
		key.CreatedAt,
		key.ExpiresAt,
	)
	return err
}

// GetByID retrieves an API key by ID.
func (r *APIKeyRepo) GetByID(ctx context.Context, id string) (*domain.APIKey, error) {
	query := `
		SELECT id, project_id, hash, prefix, status, scopes, created_at, expires_at
		FROM api_keys
		WHERE id = $1
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, id))
}

// GetByHash retrieves an API key by its SHA-256 hash.
func (r *APIKeyRepo) GetByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	query := `
		SELECT id, project_id, hash, prefix, status, scopes, created_at, expires_at
		FROM api_keys
		WHERE hash = $1
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, hash))
}

// ListByProject returns all keys associated with a project.
func (r *APIKeyRepo) ListByProject(ctx context.Context, projectID string) ([]*domain.APIKey, error) {
	query := `
		SELECT id, project_id, hash, prefix, status, scopes, created_at, expires_at
		FROM api_keys
		WHERE project_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		var status string
		var scopesStr []string
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Hash, &k.Prefix, &status, &scopesStr, &k.CreatedAt, &k.ExpiresAt); err != nil {
			return nil, err
		}
		k.Status = domain.KeyStatus(status)
		for _, s := range scopesStr {
			k.Scopes = append(k.Scopes, domain.Scope(s))
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

// Revoke transitions an API key to revoked status.
func (r *APIKeyRepo) Revoke(ctx context.Context, id string) error {
	query := `
		UPDATE api_keys
		SET status = 'revoked'
		WHERE id = $1
	`
	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *APIKeyRepo) scanRow(row pgx.Row) (*domain.APIKey, error) {
	var k domain.APIKey
	var status string
	var scopesStr []string
	err := row.Scan(
		&k.ID,
		&k.ProjectID,
		&k.Hash,
		&k.Prefix,
		&status,
		&scopesStr,
		&k.CreatedAt,
		&k.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	k.Status = domain.KeyStatus(status)
	for _, s := range scopesStr {
		k.Scopes = append(k.Scopes, domain.Scope(s))
	}
	return &k, nil
}
