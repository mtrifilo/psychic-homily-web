-- Fix discovery-imported show times: they were stored as if venue-local times
-- were UTC. All currently configured discovery venues are in AZ (UTC-7, no DST),
-- so we add 7 hours to convert from "local time stored as UTC" to actual UTC.
UPDATE shows
SET event_date = event_date + INTERVAL '7 hours'
WHERE source = 'discovery';
