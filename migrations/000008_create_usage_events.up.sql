CREATE TABLE IF NOT EXISTS usage_events (
    event_id            TEXT PRIMARY KEY,
    request_id          TEXT NOT NULL,
    project_id          TEXT NOT NULL REFERENCES projects(id),
    provider            TEXT NOT NULL,
    model               TEXT NOT NULL,
    routing_strategy    TEXT NOT NULL DEFAULT '',
    latency_ms          BIGINT NOT NULL DEFAULT 0,
    input_tokens        BIGINT NOT NULL DEFAULT 0,
    output_tokens       BIGINT NOT NULL DEFAULT 0,
    total_tokens        BIGINT NOT NULL DEFAULT 0,
    estimated_cost_usd  NUMERIC(16, 10) NOT NULL DEFAULT 0,
    fallback_used       BOOLEAN NOT NULL DEFAULT FALSE,
    retry_count         INTEGER NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'success',
    timestamp           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_events_project_id_timestamp ON usage_events (project_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_usage_events_provider_model ON usage_events (provider, model, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_usage_events_request_id ON usage_events (request_id);
