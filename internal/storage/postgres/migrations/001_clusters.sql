CREATE TABLE clusters (
    id              TEXT PRIMARY KEY,
    fingerprint     TEXT NOT NULL UNIQUE,
    template        TEXT NOT NULL,
    first_seen      TIMESTAMPTZ NOT NULL,
    last_seen       TIMESTAMPTZ NOT NULL,
    count           BIGINT NOT NULL DEFAULT 0,
    priority        SMALLINT NOT NULL DEFAULT 0,
    anomaly_flags   TEXT NOT NULL DEFAULT '[]',
    services        TEXT NOT NULL DEFAULT '[]',
    levels_json     TEXT NOT NULL DEFAULT '{}',
    examples_sample TEXT NOT NULL DEFAULT '[]',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX clusters_priority_last_seen ON clusters (priority DESC, last_seen DESC);
CREATE INDEX clusters_last_seen ON clusters (last_seen DESC);
