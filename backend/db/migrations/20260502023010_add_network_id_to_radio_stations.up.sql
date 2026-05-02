-- PSY-508: Add nullable network_id FK to radio_stations.
-- ON DELETE SET NULL keeps stations alive if the parent network is removed.
-- Index is created CONCURRENTLY in the next migration (PSY-413 rule:
-- CONCURRENTLY must be the only statement in its file).

ALTER TABLE radio_stations
    ADD COLUMN network_id BIGINT REFERENCES radio_networks(id) ON DELETE SET NULL;
