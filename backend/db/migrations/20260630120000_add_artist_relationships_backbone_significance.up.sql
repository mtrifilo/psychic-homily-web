-- PSY-1293: denormalize the disparity-filter backbone significance onto the derived
-- radio_cooccurrence artist_relationships rows, so the scene + ego graph endpoints can
-- filter on it at query time.
--
-- backbone_significance is computed on radio_artist_affinity (PSY-1261, Serrano et al. PNAS 2009)
-- and copied here by SyncAffinityToRelationships for the radio_cooccurrence relationship type only
-- (it is meaningless for the other types and stays NULL there). Both graph read paths
-- (scene: queryRelationshipsAmongArtists; ego: GetArtistGraph) read artist_relationships, so storing
-- the value here lets both consume it without a join back to the affinity table.
--
-- Storing the significance (not a boolean) keeps the alpha threshold (RADIO_BACKBONE_ALPHA) tunable
-- at query time without a recompute. LOWER = stronger; an edge is in the scene backbone at level
-- alpha iff backbone_significance < alpha. NULL = not in the scene backbone (not yet computed, a
-- non-radio relationship type, or an isolated/degenerate edge).
ALTER TABLE artist_relationships ADD COLUMN backbone_significance REAL;

-- Partial composite indexes matching the ego-graph backbone-union access (PSY-1293): the center
-- artist's radio_cooccurrence edges with a computed significance below alpha, reached from either
-- endpoint. Scoped to relationship_type = 'radio_cooccurrence' so they index only the dense radio
-- subset and stay NULL/absent for every other relationship type. The pre-existing single-column
-- idx_artist_relationships_source/target already serve the scene query's artist-set membership; the
-- significance column is a residual filter there.
CREATE INDEX idx_artist_rel_radio_backbone_source
    ON artist_relationships (source_artist_id, backbone_significance)
    WHERE relationship_type = 'radio_cooccurrence' AND backbone_significance IS NOT NULL;

CREATE INDEX idx_artist_rel_radio_backbone_target
    ON artist_relationships (target_artist_id, backbone_significance)
    WHERE relationship_type = 'radio_cooccurrence' AND backbone_significance IS NOT NULL;
