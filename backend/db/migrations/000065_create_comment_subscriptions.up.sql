-- Comment subscriptions: track which users are subscribed to comment threads on entities
CREATE TABLE comment_subscriptions (
  user_id INTEGER NOT NULL REFERENCES users(id),
  entity_type VARCHAR(20) NOT NULL,
  entity_id INTEGER NOT NULL,
  subscribed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, entity_type, entity_id)
);

-- "Who is subscribed to this entity?" lookups (for notification fan-out)
CREATE INDEX idx_comment_subscriptions_entity ON comment_subscriptions(entity_type, entity_id);

-- Comment last-read tracking: stores the highest comment ID a user has seen per entity
CREATE TABLE comment_last_read (
  user_id INTEGER NOT NULL REFERENCES users(id),
  entity_type VARCHAR(20) NOT NULL,
  entity_id INTEGER NOT NULL,
  last_read_comment_id INTEGER NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, entity_type, entity_id)
);
