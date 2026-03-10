-- collections table
CREATE TABLE collections (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    collaborative BOOLEAN NOT NULL DEFAULT true,
    cover_image_url VARCHAR(500),
    is_public BOOLEAN NOT NULL DEFAULT true,
    is_featured BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_collections_creator_id ON collections(creator_id);
CREATE INDEX idx_collections_slug ON collections(slug);
CREATE INDEX idx_collections_is_public ON collections(is_public);
CREATE INDEX idx_collections_is_featured ON collections(is_featured);

-- collection_items table
CREATE TABLE collection_items (
    id BIGSERIAL PRIMARY KEY,
    collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    entity_type VARCHAR(50) NOT NULL,
    entity_id BIGINT NOT NULL,
    position INT NOT NULL DEFAULT 0,
    added_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(collection_id, entity_type, entity_id)
);

CREATE INDEX idx_collection_items_collection_id ON collection_items(collection_id);
CREATE INDEX idx_collection_items_entity ON collection_items(entity_type, entity_id);

-- collection_subscribers table
CREATE TABLE collection_subscribers (
    collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_visited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (collection_id, user_id)
);

CREATE INDEX idx_collection_subscribers_user_id ON collection_subscribers(user_id);
