package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// RequestRecordRepo manages persistence of RequestRecord audit entries.
type RequestRecordRepo struct {
	pool *pgxpool.Pool
}

// NewRequestRecordRepo returns a new RequestRecordRepo.
func NewRequestRecordRepo(pool *pgxpool.Pool) *RequestRecordRepo {
	return &RequestRecordRepo{pool: pool}
}

// Create stores a request record in the database.
func (r *RequestRecordRepo) Create(ctx context.Context, rec *domain.RequestRecord) error {
	query := `
		INSERT INTO request_records (
			id, request_id, project_id, provider, model, routing_strategy,
			status, latency_ms, input_tokens, output_tokens, total_tokens,
			estimated_cost_usd, fallback_used, retry_count, error_code, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (request_id) DO UPDATE SET
			provider = EXCLUDED.provider,
			model = EXCLUDED.model,
			status = EXCLUDED.status,
			latency_ms = EXCLUDED.latency_ms,
			input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			total_tokens = EXCLUDED.total_tokens,
			estimated_cost_usd = EXCLUDED.estimated_cost_usd,
			fallback_used = EXCLUDED.fallback_used,
			retry_count = EXCLUDED.retry_count,
			error_code = EXCLUDED.error_code
	`
	_, err := r.pool.Exec(ctx, query,
		rec.ID,
		rec.RequestID,
		rec.ProjectID,
		string(rec.Provider),
		rec.Model,
		string(rec.RoutingStrategy),
		string(rec.Status),
		rec.LatencyMS,
		rec.InputTokens,
		rec.OutputTokens,
		rec.TotalTokens,
		rec.EstimatedCostUSD,
		rec.FallbackUsed,
		rec.RetryCount,
		rec.ErrorCode,
		rec.CreatedAt,
	)
	return err
}

// GetByRequestID retrieves a record by its public request ID.
func (r *RequestRecordRepo) GetByRequestID(ctx context.Context, requestID string) (*domain.RequestRecord, error) {
	query := `
		SELECT id, request_id, project_id, provider, model, routing_strategy,
		       status, latency_ms, input_tokens, output_tokens, total_tokens,
		       estimated_cost_usd, fallback_used, retry_count, error_code, created_at
		FROM request_records
		WHERE request_id = $1
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, requestID))
}

// ListByProject returns a paginated list of records for a project.
func (r *RequestRecordRepo) ListByProject(ctx context.Context, projectID string, limit, offset int) ([]*domain.RequestRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, request_id, project_id, provider, model, routing_strategy,
		       status, latency_ms, input_tokens, output_tokens, total_tokens,
		       estimated_cost_usd, fallback_used, retry_count, error_code, created_at
		FROM request_records
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*domain.RequestRecord
	for rows.Next() {
		rec, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *RequestRecordRepo) scanRow(row pgx.Row) (*domain.RequestRecord, error) {
	var rec domain.RequestRecord
	var prov, strat, status string
	err := row.Scan(
		&rec.ID,
		&rec.RequestID,
		&rec.ProjectID,
		&prov,
		&rec.Model,
		&strat,
		&status,
		&rec.LatencyMS,
		&rec.InputTokens,
		&rec.OutputTokens,
		&rec.TotalTokens,
		&rec.EstimatedCostUSD,
		&rec.FallbackUsed,
		&rec.RetryCount,
		&rec.ErrorCode,
		&rec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	rec.Provider = domain.ProviderID(prov)
	rec.RoutingStrategy = domain.Strategy(strat)
	rec.Status = domain.RequestStatus(status)
	return &rec, nil
}
