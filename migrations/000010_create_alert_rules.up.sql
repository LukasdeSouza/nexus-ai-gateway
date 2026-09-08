CREATE TABLE IF NOT EXISTS alert_rules (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    condition   JSONB NOT NULL DEFAULT '{}',
    channel     JSONB NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_project_id ON alert_rules (project_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules (enabled);
