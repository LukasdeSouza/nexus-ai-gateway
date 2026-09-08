CREATE TABLE IF NOT EXISTS budgets (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    period      TEXT NOT NULL DEFAULT 'monthly',
    limit_type  TEXT NOT NULL DEFAULT 'usd',
    limit_value NUMERIC(16, 6) NOT NULL,
    action      TEXT NOT NULL DEFAULT 'reject',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, period, limit_type)
);

CREATE INDEX IF NOT EXISTS idx_budgets_project_id ON budgets (project_id);
