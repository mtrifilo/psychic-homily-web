-- Add data provenance columns to all 6 core entity tables
-- data_source: where the data came from (user, ai_extraction, musicbrainz, etc.)
-- source_confidence: confidence score 0.00-1.00
-- last_verified_at: when the data was last verified/refreshed

-- Shows
ALTER TABLE shows
    ADD COLUMN data_source VARCHAR(50),
    ADD COLUMN source_confidence NUMERIC(3,2),
    ADD COLUMN last_verified_at TIMESTAMPTZ;

ALTER TABLE shows
    ADD CONSTRAINT chk_shows_source_confidence
    CHECK (source_confidence >= 0 AND source_confidence <= 1);

CREATE INDEX idx_shows_data_source ON shows (data_source);
CREATE INDEX idx_shows_last_verified_at ON shows (last_verified_at);

-- Backfill shows.data_source from existing source column
UPDATE shows SET data_source = source;

-- Artists
ALTER TABLE artists
    ADD COLUMN data_source VARCHAR(50),
    ADD COLUMN source_confidence NUMERIC(3,2),
    ADD COLUMN last_verified_at TIMESTAMPTZ;

ALTER TABLE artists
    ADD CONSTRAINT chk_artists_source_confidence
    CHECK (source_confidence >= 0 AND source_confidence <= 1);

CREATE INDEX idx_artists_data_source ON artists (data_source);
CREATE INDEX idx_artists_last_verified_at ON artists (last_verified_at);

-- Venues
ALTER TABLE venues
    ADD COLUMN data_source VARCHAR(50),
    ADD COLUMN source_confidence NUMERIC(3,2),
    ADD COLUMN last_verified_at TIMESTAMPTZ;

ALTER TABLE venues
    ADD CONSTRAINT chk_venues_source_confidence
    CHECK (source_confidence >= 0 AND source_confidence <= 1);

CREATE INDEX idx_venues_data_source ON venues (data_source);
CREATE INDEX idx_venues_last_verified_at ON venues (last_verified_at);

-- Releases
ALTER TABLE releases
    ADD COLUMN data_source VARCHAR(50),
    ADD COLUMN source_confidence NUMERIC(3,2),
    ADD COLUMN last_verified_at TIMESTAMPTZ;

ALTER TABLE releases
    ADD CONSTRAINT chk_releases_source_confidence
    CHECK (source_confidence >= 0 AND source_confidence <= 1);

CREATE INDEX idx_releases_data_source ON releases (data_source);
CREATE INDEX idx_releases_last_verified_at ON releases (last_verified_at);

-- Labels
ALTER TABLE labels
    ADD COLUMN data_source VARCHAR(50),
    ADD COLUMN source_confidence NUMERIC(3,2),
    ADD COLUMN last_verified_at TIMESTAMPTZ;

ALTER TABLE labels
    ADD CONSTRAINT chk_labels_source_confidence
    CHECK (source_confidence >= 0 AND source_confidence <= 1);

CREATE INDEX idx_labels_data_source ON labels (data_source);
CREATE INDEX idx_labels_last_verified_at ON labels (last_verified_at);

-- Festivals
ALTER TABLE festivals
    ADD COLUMN data_source VARCHAR(50),
    ADD COLUMN source_confidence NUMERIC(3,2),
    ADD COLUMN last_verified_at TIMESTAMPTZ;

ALTER TABLE festivals
    ADD CONSTRAINT chk_festivals_source_confidence
    CHECK (source_confidence >= 0 AND source_confidence <= 1);

CREATE INDEX idx_festivals_data_source ON festivals (data_source);
CREATE INDEX idx_festivals_last_verified_at ON festivals (last_verified_at);
