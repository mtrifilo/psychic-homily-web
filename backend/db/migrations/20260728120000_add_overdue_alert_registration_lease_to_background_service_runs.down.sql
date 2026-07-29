-- Reverses PSY-1612's alert-throttle column.
--
-- ROLL THE APPLICATION BACK FIRST. Scheduling's READ path is unaffected (DueIn
-- and Claim never reference these columns), but its WRITE path is not:
-- GormRunStore.Complete writes last_overdue_alert_at, so against a still-deployed
-- PSY-1612 binary every cycle completion fails on "column does not exist".
-- last_completed_at then never advances and last_started_at is never released, so
-- each loop burns its full lease before it can re-claim -- the schedule PSY-1611
-- exists to preserve, frozen, plus a catch-up cycle against MusicBrainz /
-- Nominatim / Spotify on every deploy.
--
-- Alerting recovers on its own after a down/up: the running health check
-- re-stamps last_registered_at for every loop it owns within one pass
-- (SWEEP_HEALTH_CHECK_INTERVAL_MINUTES, default 15m). No restart required.

ALTER TABLE background_service_runs
    DROP COLUMN IF EXISTS last_overdue_alert_at,
    DROP COLUMN IF EXISTS last_registered_at,
    DROP COLUMN IF EXISTS lease_seconds;
