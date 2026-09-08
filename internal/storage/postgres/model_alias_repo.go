package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// ModelAliasRepo manages persistence of ModelAlias records.
type ModelAliasRepo struct {
	pool *pgxpool.Pool
}

// NewModelAliasRepo returns a new ModelAliasRepo.
func NewModelAliasRepo(pool *pgxpool.Pool) *ModelAliasRepo {
	return &ModelAliasRepo{pool: pool}
}

// Create inserts a new model alias.
func (r *ModelAliasRepo) Create(ctx context.Context, m *domain.ModelAlias) error {
	query := `
		INSERT INTO model_aliases (
			id, alias, provider, provider_model, chat, streaming,
			function_call, vision, max_context_tokens, input_price_per_mtoken,
			output_price_per_mtoken, active, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (alias) DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query,
		m.ID,
		m.Alias,
		string(m.Provider),
		m.ProviderModel,
		m.Capabilities.Chat,
		m.Capabilities.Streaming,
		m.Capabilities.FunctionCall,
		m.Capabilities.Vision,
		m.Capabilities.MaxContextTokens,
		m.Capabilities.InputPricePerMToken,
		m.Capabilities.OutputPricePerMToken,
		m.Active,
		m.CreatedAt,
	)
	return err
}

// GetByAlias retrieves a model alias configuration.
func (r *ModelAliasRepo) GetByAlias(ctx context.Context, alias string) (*domain.ModelAlias, error) {
	query := `
		SELECT id, alias, provider, provider_model, chat, streaming,
		       function_call, vision, max_context_tokens, input_price_per_mtoken,
		       output_price_per_mtoken, active, created_at
		FROM model_aliases
		WHERE alias = $1
	`
	return r.scanRow(r.pool.QueryRow(ctx, query, alias))
}

// ListActive retrieves all currently active model aliases.
func (r *ModelAliasRepo) ListActive(ctx context.Context) ([]*domain.ModelAlias, error) {
	query := `
		SELECT id, alias, provider, provider_model, chat, streaming,
		       function_call, vision, max_context_tokens, input_price_per_mtoken,
		       output_price_per_mtoken, active, created_at
		FROM model_aliases
		WHERE active = TRUE
		ORDER BY alias ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aliases []*domain.ModelAlias
	for rows.Next() {
		m, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		aliases = append(aliases, m)
	}
	return aliases, rows.Err()
}

// SeedDefaults populates default model aliases if table is empty.
func (r *ModelAliasRepo) SeedDefaults(ctx context.Context) error {
	defaults := domain.DefaultModelAliases()
	for _, m := range defaults {
		aliasCopy := m
		if err := r.Create(ctx, &aliasCopy); err != nil {
			return err
		}
	}
	return nil
}

func (r *ModelAliasRepo) scanRow(row pgx.Row) (*domain.ModelAlias, error) {
	var m domain.ModelAlias
	var prov string
	err := row.Scan(
		&m.ID,
		&m.Alias,
		&prov,
		&m.ProviderModel,
		&m.Capabilities.Chat,
		&m.Capabilities.Streaming,
		&m.Capabilities.FunctionCall,
		&m.Capabilities.Vision,
		&m.Capabilities.MaxContextTokens,
		&m.Capabilities.InputPricePerMToken,
		&m.Capabilities.OutputPricePerMToken,
		&m.Active,
		&m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	m.Provider = domain.ProviderID(prov)
	return &m, nil
}
