-- labels table
CREATE TABLE labels (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE,
    city VARCHAR(255),
    state VARCHAR(255),
    country VARCHAR(100),
    founded_year INT,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    description TEXT,
    website TEXT,
    instagram VARCHAR(255),
    facebook VARCHAR(255),
    twitter VARCHAR(255),
    youtube VARCHAR(255),
    spotify VARCHAR(255),
    soundcloud VARCHAR(255),
    bandcamp VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- artist_labels junction (an artist is signed to / has released on a label)
CREATE TABLE artist_labels (
    artist_id INT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    label_id INT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (artist_id, label_id)
);

-- release_labels junction (a release was put out by a label)
CREATE TABLE release_labels (
    release_id INT NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
    label_id INT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    catalog_number VARCHAR(100),
    PRIMARY KEY (release_id, label_id)
);

-- Indexes
CREATE INDEX idx_labels_slug ON labels(slug);
CREATE INDEX idx_labels_status ON labels(status);
CREATE INDEX idx_labels_city_state ON labels(city, state);
CREATE INDEX idx_artist_labels_artist_id ON artist_labels(artist_id);
CREATE INDEX idx_artist_labels_label_id ON artist_labels(label_id);
CREATE INDEX idx_release_labels_release_id ON release_labels(release_id);
CREATE INDEX idx_release_labels_label_id ON release_labels(label_id);
