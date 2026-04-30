CREATE TABLE collection_likes (
    user_id BIGINT NOT NULL,
    collection_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, collection_id),
    CONSTRAINT fk_collection_likes_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_collection_likes_collection FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE
);

CREATE INDEX idx_collection_likes_collection_id ON collection_likes(collection_id);
CREATE INDEX idx_collection_likes_user_id ON collection_likes(user_id);
