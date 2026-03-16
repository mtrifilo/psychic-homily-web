-- Notification filters: user-created filter rules for automatic show notifications
CREATE TABLE notification_filters (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    -- Match criteria (all nullable — NULL means "any")
    artist_ids BIGINT[],
    venue_ids BIGINT[],
    label_ids BIGINT[],
    tag_ids BIGINT[],
    exclude_tag_ids BIGINT[],
    cities JSONB,
    price_max_cents INT,

    -- Delivery preferences
    notify_email BOOLEAN NOT NULL DEFAULT TRUE,
    notify_in_app BOOLEAN NOT NULL DEFAULT TRUE,
    notify_push BOOLEAN NOT NULL DEFAULT FALSE,

    -- Metadata
    last_matched_at TIMESTAMPTZ,
    match_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notification_filters_user ON notification_filters(user_id) WHERE is_active = TRUE;
CREATE INDEX idx_notification_filters_artists ON notification_filters USING GIN(artist_ids) WHERE artist_ids IS NOT NULL;
CREATE INDEX idx_notification_filters_venues ON notification_filters USING GIN(venue_ids) WHERE venue_ids IS NOT NULL;
CREATE INDEX idx_notification_filters_tags ON notification_filters USING GIN(tag_ids) WHERE tag_ids IS NOT NULL;

-- Notification log: records every notification sent, for deduplication and user history
CREATE TABLE notification_log (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filter_id BIGINT REFERENCES notification_filters(id) ON DELETE SET NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id BIGINT NOT NULL,
    channel VARCHAR(20) NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at TIMESTAMPTZ,
    UNIQUE(user_id, filter_id, entity_type, entity_id, channel)
);

CREATE INDEX idx_notification_log_user ON notification_log(user_id, sent_at DESC);
CREATE INDEX idx_notification_log_unread ON notification_log(user_id) WHERE read_at IS NULL;
