-- PSY-1262: persisted Leiden community partition over the artist-similarity
-- graph. community_id is the dense per-partition index; the label table
-- carries each community's display metadata ("Around {artist}"). Both are
-- rebuilt atomically by the nightly compute, so no FK from artists into
-- artist_communities (the partition swap would race it).

ALTER TABLE artists ADD COLUMN community_id INTEGER;

CREATE INDEX idx_artists_community ON artists (community_id)
    WHERE community_id IS NOT NULL;

CREATE TABLE artist_communities (
    id INTEGER PRIMARY KEY,
    label_artist_id BIGINT NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
    member_count INTEGER NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
