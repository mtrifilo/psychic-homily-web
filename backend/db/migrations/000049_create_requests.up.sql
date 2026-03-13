CREATE TABLE requests (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    entity_type VARCHAR(50) NOT NULL,
    requested_entity_id BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    requester_id BIGINT NOT NULL REFERENCES users(id),
    fulfiller_id BIGINT REFERENCES users(id),
    vote_score INTEGER NOT NULL DEFAULT 0,
    upvotes INTEGER NOT NULL DEFAULT 0,
    downvotes INTEGER NOT NULL DEFAULT 0,
    fulfilled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_requests_entity_type ON requests(entity_type);
CREATE INDEX idx_requests_status ON requests(status);
CREATE INDEX idx_requests_requester_id ON requests(requester_id);
CREATE INDEX idx_requests_vote_score ON requests(vote_score DESC);
CREATE INDEX idx_requests_created_at ON requests(created_at DESC);

-- Composite index for browsing requests by entity type + status
CREATE INDEX idx_requests_entity_type_status ON requests(entity_type, status);

CREATE TABLE request_votes (
    request_id BIGINT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    vote SMALLINT NOT NULL CHECK (vote IN (-1, 1)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (request_id, user_id)
);

CREATE INDEX idx_request_votes_user_id ON request_votes(user_id);
