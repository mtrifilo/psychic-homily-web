-- PSY-927: Restore the WFMU flagship station's playlist_source.
--
-- The station (slug 'wfmu') had playlist_source = 'wfmu_html' — a value no
-- provider handles. getProvider() returns "unsupported playlist source" for it,
-- so episode discovery ran but every WFMU show imported 0 tracks. The seed
-- (000068_seed_default_radio_stations) set 'wfmu_scrape'; 'wfmu_html' was
-- introduced later at runtime (an unvalidated write). Code-side hardening
-- (IsValidPlaylistSource on station create/update) prevents recurrence; this
-- corrects the existing bad row.
--
-- Scoped to the exact broken value and idempotent: re-running, or running on an
-- environment that was never broken (including the fresh CI database, where the
-- seed sets 'wfmu_scrape'), updates 0 rows.
UPDATE radio_stations
   SET playlist_source = 'wfmu_scrape'
 WHERE playlist_source = 'wfmu_html';
