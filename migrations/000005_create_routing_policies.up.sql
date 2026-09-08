CREATE TABLE IF NOT EXISTS routing_policies (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    strategy        TEXT NOT NULL DEFAULT 'balanced',
    candidates      JSONB NOT NULL DEFAULT '[]',
    fallback_policy JSONB,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_routing_policies_project_id_active ON routing_policies (project_id, active);
