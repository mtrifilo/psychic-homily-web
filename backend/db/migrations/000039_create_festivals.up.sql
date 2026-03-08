-- festivals table
CREATE TABLE festivals (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    series_slug VARCHAR(255) NOT NULL,
    edition_year INT NOT NULL,
    description TEXT,
    location_name VARCHAR(255),
    city VARCHAR(100),
    state VARCHAR(100),
    country VARCHAR(100) DEFAULT 'US',
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    website VARCHAR(500),
    ticket_url VARCHAR(500),
    flyer_url VARCHAR(500),
    status VARCHAR(50) NOT NULL DEFAULT 'announced',
    social JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(series_slug, edition_year)
);

-- festival_artists junction (artist performing at a festival)
CREATE TABLE festival_artists (
    id BIGSERIAL PRIMARY KEY,
    festival_id BIGINT NOT NULL REFERENCES festivals(id) ON DELETE CASCADE,
    artist_id BIGINT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    billing_tier VARCHAR(50) NOT NULL DEFAULT 'mid_card',
    position INT NOT NULL DEFAULT 0,
    day_date DATE,
    stage VARCHAR(255),
    set_time TIME,
    venue_id BIGINT REFERENCES venues(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(festival_id, artist_id)
);

-- festival_venues junction (for multi-venue takeover festivals)
CREATE TABLE festival_venues (
    id BIGSERIAL PRIMARY KEY,
    festival_id BIGINT NOT NULL REFERENCES festivals(id) ON DELETE CASCADE,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(festival_id, venue_id)
);

-- Indexes
CREATE INDEX idx_festivals_slug ON festivals(slug);
CREATE INDEX idx_festivals_series_slug ON festivals(series_slug);
CREATE INDEX idx_festivals_city_state ON festivals(city, state);
CREATE INDEX idx_festivals_start_date ON festivals(start_date);
CREATE INDEX idx_festivals_status ON festivals(status);
CREATE INDEX idx_festival_artists_artist_id ON festival_artists(artist_id);
CREATE INDEX idx_festival_artists_festival_billing ON festival_artists(festival_id, billing_tier);
CREATE INDEX idx_festival_venues_venue_id ON festival_venues(venue_id);
