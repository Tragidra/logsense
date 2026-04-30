-- Drain template generalization mutates a cluster's fingerprint over time as
-- new events match an existing group, so fingerprint cannot be a stable key.
-- Cluster ID (assigned once when a LogGroup is created) is the upsert target.
-- Drop the UNIQUE constraint by recreating the table without it; replace it
-- with a non-unique index for lookup performance.

CREATE TABLE clusters_new (
    id              TEXT PRIMARY KEY,
    fingerprint     TEXT NOT NULL,
    template        TEXT NOT NULL,
    first_seen      TEXT NOT NULL,
    last_seen       TEXT NOT NULL,
    count           INTEGER NOT NULL DEFAULT 0,
    priority        INTEGER NOT NULL DEFAULT 0,
    anomaly_flags   TEXT NOT NULL DEFAULT '[]',
    services        TEXT NOT NULL DEFAULT '[]',
    levels_json     TEXT NOT NULL DEFAULT '{}',
    examples_sample TEXT NOT NULL DEFAULT '[]',
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT INTO clusters_new SELECT * FROM clusters;

DROP TABLE clusters;
ALTER TABLE clusters_new RENAME TO clusters;

CREATE INDEX clusters_priority_last_seen ON clusters (priority DESC, last_seen DESC);
CREATE INDEX clusters_last_seen ON clusters (last_seen DESC);
CREATE INDEX clusters_fingerprint ON clusters (fingerprint);
