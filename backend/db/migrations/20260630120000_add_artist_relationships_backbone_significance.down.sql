DROP INDEX IF EXISTS idx_artist_rel_radio_backbone_target;
DROP INDEX IF EXISTS idx_artist_rel_radio_backbone_source;
ALTER TABLE artist_relationships DROP COLUMN IF EXISTS backbone_significance;
