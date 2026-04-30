CREATE TABLE events (
    id          TEXT NOT NULL,
    cluster_id  TEXT NOT NULL,
    ts          TIMESTAMPTZ NOT NULL,
    level       SMALLINT NOT NULL,
    service     TEXT,
    message     TEXT NOT NULL,
    fields_json TEXT,
    raw         TEXT,
    source      TEXT NOT NULL,
    trace_id    TEXT,
    PRIMARY KEY (ts, id)
);

CREATE INDEX events_cluster_ts ON events (cluster_id, ts DESC);
