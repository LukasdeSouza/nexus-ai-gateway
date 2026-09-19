-- 000011_create_tasks.up.sql
CREATE TABLE IF NOT EXISTS tasks (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    prompt          TEXT NOT NULL,
    preset          TEXT NOT NULL DEFAULT 'auto',
    policy          TEXT NOT NULL DEFAULT 'approve',
    budget          NUMERIC NOT NULL DEFAULT 0.0,
    status          TEXT NOT NULL DEFAULT 'pending',
    result_summary  TEXT,
    diff            TEXT,
    claimed_by      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tasks_project_id_status ON tasks (project_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status);
