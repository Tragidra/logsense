ALTER TABLE clusters DROP CONSTRAINT clusters_fingerprint_key;
CREATE INDEX clusters_fingerprint ON clusters (fingerprint);
