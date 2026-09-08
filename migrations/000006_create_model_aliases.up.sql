CREATE TABLE IF NOT EXISTS model_aliases (
    id                      TEXT PRIMARY KEY,
    alias                   TEXT NOT NULL UNIQUE,
    provider                TEXT NOT NULL,
    provider_model          TEXT NOT NULL,
    chat                    BOOLEAN NOT NULL DEFAULT TRUE,
    streaming               BOOLEAN NOT NULL DEFAULT TRUE,
    function_call           BOOLEAN NOT NULL DEFAULT FALSE,
    vision                  BOOLEAN NOT NULL DEFAULT FALSE,
    max_context_tokens      INTEGER NOT NULL DEFAULT 128000,
    input_price_per_mtoken  NUMERIC(12, 8) NOT NULL DEFAULT 0,
    output_price_per_mtoken NUMERIC(12, 8) NOT NULL DEFAULT 0,
    active                  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_model_aliases_provider ON model_aliases (provider);
CREATE INDEX IF NOT EXISTS idx_model_aliases_active ON model_aliases (active);
