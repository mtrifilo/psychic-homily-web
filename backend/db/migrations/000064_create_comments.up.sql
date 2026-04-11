-- Comments table: polymorphic discussion system for all entity types
CREATE TABLE comments (
  id SERIAL PRIMARY KEY,
  entity_type VARCHAR(20) NOT NULL,
  entity_id INTEGER NOT NULL,
  kind VARCHAR(20) NOT NULL DEFAULT 'comment',
  user_id INTEGER NOT NULL REFERENCES users(id),
  parent_id INTEGER REFERENCES comments(id),
  root_id INTEGER REFERENCES comments(id),
  depth INTEGER NOT NULL DEFAULT 0,
  body TEXT NOT NULL,
  body_html TEXT NOT NULL,
  structured_data JSONB,
  visibility VARCHAR(20) NOT NULL DEFAULT 'visible',
  reply_permission VARCHAR(20) NOT NULL DEFAULT 'anyone',
  ups INTEGER NOT NULL DEFAULT 0,
  downs INTEGER NOT NULL DEFAULT 0,
  score DOUBLE PRECISION NOT NULL DEFAULT 0,
  edit_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Primary query: list comments for an entity sorted by score
CREATE INDEX idx_comments_entity ON comments(entity_type, entity_id);

-- Thread loading: load all replies for a root comment
CREATE INDEX idx_comments_root ON comments(root_id);

-- Reply loading: load immediate children
CREATE INDEX idx_comments_parent ON comments(parent_id);

-- User's comments
CREATE INDEX idx_comments_user ON comments(user_id);

-- Field notes queries: filter by kind on a specific entity
CREATE INDEX idx_comments_entity_kind ON comments(entity_type, entity_id, kind);

-- Comment edits table: append-only edit history
CREATE TABLE comment_edits (
  id SERIAL PRIMARY KEY,
  comment_id INTEGER NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  old_body TEXT NOT NULL,
  edited_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_comment_edits_comment ON comment_edits(comment_id);

-- Comment votes table: binary up/down votes
CREATE TABLE comment_votes (
  comment_id INTEGER NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id),
  direction SMALLINT NOT NULL CHECK (direction IN (-1, 1)),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (comment_id, user_id)
);
