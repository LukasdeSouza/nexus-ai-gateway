CREATE TABLE IF NOT EXISTS provider_connections (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,
    secret_ref  TEXT NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_provider_connections_project_id ON provider_connections (project_id);
CREATE INDEX IF NOT EXISTS idx_provider_connections_enabled ON provider_connections (enabled);
