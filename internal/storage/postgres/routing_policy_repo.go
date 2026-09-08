package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// RoutingPolicyRepo manages persistence of RoutingPolicy records.
type RoutingPolicyRepo struct {
	pool *pgxpool.Pool
}

// NewRoutingPolicyRepo returns a new RoutingPolicyRepo.
func NewRoutingPolicyRepo(pool *pgxpool.Pool) *RoutingPolicyRepo {
	return &RoutingPolicyRepo{pool: pool}
}

// Create inserts a routing policy.
func (r *RoutingPolicyRepo) Create(ctx context.Context, pol *domain.RoutingPolicy) error {
	candBytes, err := json.Marshal(pol.Candidates)
	if err != nil {
		return err
	}
	var fallbackBytes []byte
	if pol.FallbackPolicy != nil {
		fallbackBytes, err = json.Marshal(pol.FallbackPolicy)
		if err != nil {
			return err
		}
	}

	query := `
		INSERT INTO routing_policies (id, project_id, name, strategy, candidates, fallback_policy, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = r.pool.Exec(ctx, query,
		pol.ID,
		pol.ProjectID,
		pol.Name,
		string(pol.Strategy),
		candBytes,
		fallbackBytes,
		pol.Active,
		pol.CreatedAt,
		pol.UpdatedAt,
	)
	return err
}

// GetByID retrieves a policy by ID.
func (r *RoutingPolicyRepo) GetByID(ctx context.Context, id string) (*domain.RoutingPolicy, error) {
	query := `
		SELECT id, project_id, name, strategy, candidates, fallback_policy, active, created_at, updated_at
		FROM routing_policies
		WHERE id = $1
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, id))
}

// GetActiveByProject retrieves the currently active routing policy for a project.
func (r *RoutingPolicyRepo) GetActiveByProject(ctx context.Context, projectID string) (*domain.RoutingPolicy, error) {
	query := `
		SELECT id, project_id, name, strategy, candidates, fallback_policy, active, created_at, updated_at
		FROM routing_policies
		WHERE project_id = $1 AND active = TRUE
		ORDER BY created_at DESC
		LIMIT 1
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, projectID))
}

// Deactivate sets active = FALSE for a policy.
func (r *RoutingPolicyRepo) Deactivate(ctx context.Context, id string) error {
	query := `
		UPDATE routing_policies
		SET active = FALSE, updated_at = $1
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

func (r *RoutingPolicyRepo) scanRow(row pgx.Row) (*domain.RoutingPolicy, error) {
	var pol domain.RoutingPolicy
	var strat string
	var candBytes, fallbackBytes []byte

	err := row.Scan(
		&pol.ID,
		&pol.ProjectID,
		&pol.Name,
		&strat,
		&candBytes,
		&fallbackBytes,
		&pol.Active,
		&pol.CreatedAt,
		&pol.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	pol.Strategy = domain.Strategy(strat)
	if len(candBytes) > 0 {
		_ = json.Unmarshal(candBytes, &pol.Candidates)
	}
	if len(fallbackBytes) > 0 {
		var fp domain.FallbackPolicy
		if json.Unmarshal(fallbackBytes, &fp) == nil {
			pol.FallbackPolicy = &fp
		}
	}

	return &pol, nil
}
