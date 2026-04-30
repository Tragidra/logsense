CREATE TABLE analyses (
    id                  TEXT PRIMARY KEY,
    cluster_id          TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    window_start        TEXT NOT NULL,
    window_end          TEXT NOT NULL,
    summary             TEXT NOT NULL,
    severity            INTEGER NOT NULL,
    root_cause          TEXT,
    suggested_actions   TEXT NOT NULL DEFAULT '[]',
    related_cluster_ids TEXT NOT NULL DEFAULT '[]',
    confidence          REAL,
    model_used          TEXT NOT NULL,
    tokens_input        INTEGER,
    tokens_output       INTEGER,
    latency_ms          INTEGER,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (cluster_id, window_start, window_end)
);

CREATE INDEX analyses_cluster_window_end ON analyses (cluster_id, window_end DESC);
