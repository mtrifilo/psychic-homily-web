-- PSY-669: add is_flagship to radio_stations so the network nesting UI
-- can derive which station of a network is the primary/default view
-- (e.g. WFMU 91.1 is the flagship of the WFMU network; the 3 stream-only
-- sub-channels are non-flagship siblings). Backfills WFMU only — every
-- other existing station has no network_id today, so flagship is moot.
ALTER TABLE radio_stations
  ADD COLUMN is_flagship BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE radio_stations
   SET is_flagship = TRUE
 WHERE slug = 'wfmu';
