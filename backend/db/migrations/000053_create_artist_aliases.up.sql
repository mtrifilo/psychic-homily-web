CREATE TABLE artist_aliases (
    id BIGSERIAL PRIMARY KEY,
    artist_id BIGINT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    alias VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_artist_aliases_alias_lower ON artist_aliases(LOWER(alias));
CREATE INDEX idx_artist_aliases_artist ON artist_aliases(artist_id);
