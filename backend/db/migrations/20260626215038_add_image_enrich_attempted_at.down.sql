ALTER TABLE artists  DROP COLUMN IF EXISTS image_enrich_attempted_at;
ALTER TABLE releases DROP COLUMN IF EXISTS image_enrich_attempted_at;
