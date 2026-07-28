-- venue_confirmations: one row per (user, venue) "this info is still accurate".
--
-- Composite PK makes the confirmation inherently unique, so the write is
-- INSERT ... ON CONFLICT DO NOTHING and a repeat tap is an idempotent no-op
-- rather than an error. Same shape as collection_likes: no counter column on
-- the parent row -- counts are aggregated at read time.
CREATE TABLE venue_confirmations (
    user_id BIGINT NOT NULL,
    venue_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, venue_id),
    CONSTRAINT fk_venue_confirmations_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_venue_confirmations_venue FOREIGN KEY (venue_id) REFERENCES venues(id) ON DELETE CASCADE
);

-- The provenance read model aggregates by venue for a whole page of venues at
-- once (venue_id IN (...)), so venue_id leads its own index; created_at rides
-- along so "latest confirmation per venue" is answered from the index.
CREATE INDEX idx_venue_confirmations_venue_id ON venue_confirmations (venue_id, created_at DESC);
CREATE INDEX idx_venue_confirmations_user_id ON venue_confirmations (user_id);
