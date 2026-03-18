-- Radio entities: stations, shows, episodes, plays, and artist affinity

CREATE TABLE radio_stations (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    city VARCHAR(100),
    state VARCHAR(100),
    country VARCHAR(100) DEFAULT 'US',
    timezone VARCHAR(50),
    stream_url TEXT,
    stream_urls JSONB DEFAULT '{}',
    website VARCHAR(500),
    donation_url VARCHAR(500),
    donation_embed_url VARCHAR(500),
    logo_url VARCHAR(500),
    social JSONB DEFAULT '{}',
    broadcast_type VARCHAR(20) NOT NULL DEFAULT 'both',
    frequency_mhz DECIMAL(5,1),
    playlist_source VARCHAR(50),
    playlist_config JSONB,
    last_playlist_fetch_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE radio_shows (
    id BIGSERIAL PRIMARY KEY,
    station_id BIGINT NOT NULL REFERENCES radio_stations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    host_name VARCHAR(255),
    description TEXT,
    schedule_display VARCHAR(255),
    schedule JSONB,
    genre_tags JSONB DEFAULT '[]',
    archive_url VARCHAR(500),
    image_url VARCHAR(500),
    external_id VARCHAR(255),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(station_id, external_id)
);

CREATE INDEX idx_radio_shows_station ON radio_shows(station_id);
CREATE INDEX idx_radio_shows_active ON radio_shows(station_id) WHERE is_active = TRUE;

CREATE TABLE radio_episodes (
    id BIGSERIAL PRIMARY KEY,
    show_id BIGINT NOT NULL REFERENCES radio_shows(id) ON DELETE CASCADE,
    title VARCHAR(255),
    air_date DATE NOT NULL,
    air_time TIME,
    duration_minutes INT,
    description TEXT,
    archive_url VARCHAR(500),
    mixcloud_url VARCHAR(500),
    external_id VARCHAR(255),
    genre_tags JSONB,
    mood_tags JSONB,
    play_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(show_id, air_date, COALESCE(external_id, ''))
);

CREATE INDEX idx_radio_episodes_show ON radio_episodes(show_id, air_date DESC);
CREATE INDEX idx_radio_episodes_date ON radio_episodes(air_date DESC);

CREATE TABLE radio_plays (
    id BIGSERIAL PRIMARY KEY,
    episode_id BIGINT NOT NULL REFERENCES radio_episodes(id) ON DELETE CASCADE,
    position INT NOT NULL DEFAULT 0,

    -- Raw metadata from source (always stored, never overwritten)
    artist_name VARCHAR(500) NOT NULL,
    track_title VARCHAR(500),
    album_title VARCHAR(500),
    label_name VARCHAR(500),
    release_year INT,

    -- Curation signals
    is_new BOOLEAN NOT NULL DEFAULT FALSE,
    rotation_status VARCHAR(50),
    dj_comment TEXT,
    is_live_performance BOOLEAN NOT NULL DEFAULT FALSE,
    is_request BOOLEAN NOT NULL DEFAULT FALSE,

    -- Linked to our knowledge graph (populated by matching engine, nullable)
    artist_id INT REFERENCES artists(id) ON DELETE SET NULL,
    release_id INT REFERENCES releases(id) ON DELETE SET NULL,
    label_id INT REFERENCES labels(id) ON DELETE SET NULL,

    -- External IDs for cross-referencing and deduplication
    musicbrainz_recording_id VARCHAR(36),
    musicbrainz_artist_id VARCHAR(36),
    musicbrainz_release_id VARCHAR(36),

    -- Timing
    air_timestamp TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_radio_plays_episode ON radio_plays(episode_id, position);
CREATE INDEX idx_radio_plays_artist_id ON radio_plays(artist_id) WHERE artist_id IS NOT NULL;
CREATE INDEX idx_radio_plays_release_id ON radio_plays(release_id) WHERE release_id IS NOT NULL;
CREATE INDEX idx_radio_plays_label_id ON radio_plays(label_id) WHERE label_id IS NOT NULL;
CREATE INDEX idx_radio_plays_artist_name ON radio_plays(artist_name);
CREATE INDEX idx_radio_plays_is_new ON radio_plays(episode_id) WHERE is_new = TRUE;
CREATE INDEX idx_radio_plays_mb_artist ON radio_plays(musicbrainz_artist_id) WHERE musicbrainz_artist_id IS NOT NULL;

CREATE TABLE radio_artist_affinity (
    artist_a_id INT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    artist_b_id INT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    co_occurrence_count INT NOT NULL DEFAULT 0,
    show_count INT NOT NULL DEFAULT 0,
    station_count INT NOT NULL DEFAULT 0,
    last_co_occurrence DATE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (artist_a_id, artist_b_id),
    CHECK (artist_a_id < artist_b_id)
);

CREATE INDEX idx_radio_affinity_a ON radio_artist_affinity(artist_a_id);
CREATE INDEX idx_radio_affinity_b ON radio_artist_affinity(artist_b_id);
