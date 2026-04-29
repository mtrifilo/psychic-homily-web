-- Revert PSY-350 additions.

ALTER TABLE user_preferences
    DROP COLUMN IF EXISTS notify_on_collection_digest;

ALTER TABLE collection_subscribers
    DROP COLUMN IF EXISTS last_digest_sent_at;
