-- Retire the community "suggest a match" queue for radio plays. The feature
-- was never used (zero rows in production and stage); the per-row CTA is
-- removed from the playlist UI and the API surface is deleted in the same
-- change. DROP TABLE removes indexes, CHECKs, and FKs with it.
DROP TABLE IF EXISTS radio_play_match_suggestions;
