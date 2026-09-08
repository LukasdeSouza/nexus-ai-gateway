CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    hash        TEXT NOT NULL UNIQUE,
    prefix      TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    scopes      TEXT[] NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_project_id_status ON api_keys (project_id, status);
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys (hash);
