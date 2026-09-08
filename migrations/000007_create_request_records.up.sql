CREATE TABLE IF NOT EXISTS request_records (
    id                  TEXT PRIMARY KEY,
    request_id          TEXT NOT NULL UNIQUE,
    project_id          TEXT NOT NULL REFERENCES projects(id),
    provider            TEXT NOT NULL DEFAULT '',
    model               TEXT NOT NULL DEFAULT '',
    routing_strategy    TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'success',
    latency_ms          BIGINT NOT NULL DEFAULT 0,
    input_tokens        BIGINT NOT NULL DEFAULT 0,
    output_tokens       BIGINT NOT NULL DEFAULT 0,
    total_tokens        BIGINT NOT NULL DEFAULT 0,
    estimated_cost_usd  NUMERIC(16, 10) NOT NULL DEFAULT 0,
    fallback_used       BOOLEAN NOT NULL DEFAULT FALSE,
    retry_count         INTEGER NOT NULL DEFAULT 0,
    error_code          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_request_records_project_id_created ON request_records (project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_request_records_request_id ON request_records (request_id);
CREATE INDEX IF NOT EXISTS idx_request_records_provider ON request_records (provider, created_at DESC);
