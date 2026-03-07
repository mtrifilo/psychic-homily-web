-- releases table
CREATE TABLE releases (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE,
    release_type VARCHAR(50) NOT NULL DEFAULT 'lp',
    release_year INT,
    release_date DATE,
    cover_art_url TEXT,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- artist_releases junction with roles
CREATE TABLE artist_releases (
    artist_id INT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    release_id INT NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'main',
    position INT NOT NULL DEFAULT 0,
    PRIMARY KEY (artist_id, release_id, role)
);

-- release_external_links for Listen/Buy links
CREATE TABLE release_external_links (
    id SERIAL PRIMARY KEY,
    release_id INT NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL,
    url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_releases_slug ON releases(slug);
CREATE INDEX idx_releases_release_year ON releases(release_year);
CREATE INDEX idx_releases_release_type ON releases(release_type);
CREATE INDEX idx_artist_releases_artist_id ON artist_releases(artist_id);
CREATE INDEX idx_artist_releases_release_id ON artist_releases(release_id);
CREATE INDEX idx_release_external_links_release_id ON release_external_links(release_id);
