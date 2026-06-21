-- PSY-1154: drop the post-air backfill attempt counter. DROP COLUMN removes its
-- CHECK constraint with it, so the up->down->up CI round-trip lands back on the
-- pre-PSY-1154 schema exactly.
ALTER TABLE radio_episodes
    DROP COLUMN IF EXISTS playlist_fetch_attempts;
