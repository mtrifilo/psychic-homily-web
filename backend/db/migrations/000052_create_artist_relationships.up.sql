-- Core relationship table
CREATE TABLE artist_relationships (
    source_artist_id BIGINT NOT NULL REFERENCES artists(id),
    target_artist_id BIGINT NOT NULL REFERENCES artists(id),
    relationship_type VARCHAR(20) NOT NULL,
    score REAL NOT NULL DEFAULT 0,
    auto_derived BOOLEAN NOT NULL DEFAULT false,
    detail JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_artist_id, target_artist_id, relationship_type),
    CHECK (source_artist_id < target_artist_id)
);

CREATE INDEX idx_artist_relationships_source ON artist_relationships(source_artist_id);
CREATE INDEX idx_artist_relationships_target ON artist_relationships(target_artist_id);
CREATE INDEX idx_artist_relationships_type ON artist_relationships(relationship_type);
CREATE INDEX idx_artist_relationships_score ON artist_relationships(score DESC);

-- Per-user votes on relationships
CREATE TABLE artist_relationship_votes (
    source_artist_id BIGINT NOT NULL,
    target_artist_id BIGINT NOT NULL,
    relationship_type VARCHAR(20) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id),
    direction SMALLINT NOT NULL CHECK (direction IN (-1, 1)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_artist_id, target_artist_id, relationship_type, user_id),
    FOREIGN KEY (source_artist_id, target_artist_id, relationship_type)
        REFERENCES artist_relationships(source_artist_id, target_artist_id, relationship_type)
);
