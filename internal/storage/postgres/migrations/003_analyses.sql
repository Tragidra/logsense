CREATE TABLE analyses (
    id                  TEXT PRIMARY KEY,
    cluster_id          TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    window_start        TIMESTAMPTZ NOT NULL,
    window_end          TIMESTAMPTZ NOT NULL,
    summary             TEXT NOT NULL,
    severity            SMALLINT NOT NULL,
    root_cause          TEXT,
    suggested_actions   TEXT NOT NULL DEFAULT '[]',
    related_cluster_ids TEXT NOT NULL DEFAULT '[]',
    confidence          REAL,
    model_used          TEXT NOT NULL,
    tokens_input        INT,
    tokens_output       INT,
    latency_ms          INT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, window_start, window_end)
);

CREATE INDEX analyses_cluster_window_end ON analyses (cluster_id, window_end DESC);
