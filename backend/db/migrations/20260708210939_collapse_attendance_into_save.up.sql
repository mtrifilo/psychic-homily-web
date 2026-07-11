-- Collapse show attendance (going/interested) into the single `save` action.
--
-- `interested` and `save` encoded the same user intent — keep this show on my
-- radar — while each delivered only half the payoff: `interested` fed the public
-- trending chart but no reminder, `save` fed the reminder but no public signal.
-- `going` was an unconfirmed pre-show RSVP that the (now removed) concert diary
-- reinterpreted as attendance. One action replaces all three.
--
-- user_bookmarks carries UNIQUE(user_id, entity_type, entity_id, action), so a
-- user holding two or three of going/interested/save on the same show collides
-- the moment every action becomes 'save'. Elect one survivor row per
-- (user, show), drop the rest, then relabel.
--
-- The survivor MUST prefer an existing 'save' row. reminder_sent_at is only ever
-- populated on 'save' rows (the reminder cycle filters action = 'save'), so
-- keeping that row preserves "this user was already reminded about this show"
-- and prevents a duplicate 24h reminder email after the collapse. Electing an
-- existing row rather than delete-and-reinsert also preserves every other column
-- (settings, scene_digest_sent_at, ...) without enumerating them — a column
-- added after this migration was written would otherwise be silently dropped.
--
-- No explicit BEGIN/COMMIT: golang-migrate runs each migration file inside its
-- own transaction.
--
-- The LOCK is load-bearing. Each container runs `migrate up` on boot
-- (docker-entrypoint.sh), so during a rolling deploy this migration can run
-- while the OUTGOING container is still serving the old attendance endpoints.
-- Without the lock, a `going` row inserted between the DELETE and the UPDATE
-- below would be renamed to 'save' by the UPDATE and collide with a 'save' row
-- the same user already holds — a unique violation that aborts the migration
-- and fails the new container's boot. SHARE ROW EXCLUSIVE blocks concurrent
-- INSERT/UPDATE/DELETE on the table for the duration.
--
-- lock_timeout is not optional. Without it the LOCK waits indefinitely on a
-- long-running writer, and because Postgres queues lock requests FIFO, every
-- subsequent write to user_bookmarks (saves, follows, bookmarks — across all
-- entity types, not just shows) piles up behind it. That turns a moment of
-- contention into a spreading write freeze AND hangs the container boot, since
-- the entrypoint runs `migrate up` synchronously before starting the server.
--
-- On timeout this migration fails. Be clear about what that costs: golang-migrate
-- marks the version dirty on ANY failure (verified: a failed migration leaves
-- schema_migrations.dirty = true, and the next `migrate up` refuses with
-- "Dirty database version N. Fix and force version"). Recovery is manual —
-- `migrate force <N-1>` and redeploy. That is NOT specific to the timeout: the
-- unique violation this LOCK prevents would dirty the database exactly the same
-- way. The timeout only bounds how long we wait before failing, and spares the
-- table a FIFO write freeze while we wait.
SET LOCAL lock_timeout = '5s';
LOCK TABLE user_bookmarks IN SHARE ROW EXCLUSIVE MODE;

WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY user_id, entity_id
            ORDER BY (action = 'save') DESC, created_at ASC, id ASC
        ) AS rn
    FROM user_bookmarks
    WHERE entity_type = 'show'
      AND action IN ('going', 'interested', 'save')
)
DELETE FROM user_bookmarks ub
USING ranked r
WHERE ub.id = r.id
  AND r.rn > 1;

UPDATE user_bookmarks
SET action = 'save'
WHERE entity_type = 'show'
  AND action IN ('going', 'interested');

-- The `attendance` privacy key gated the public concert diary, which this change
-- removes. Strip the now-orphaned key from stored rows and from the column
-- default. privacy_settings is NOT NULL with a DB default, so every row carries
-- explicit JSON; jsonb_exists is used rather than the `?` operator to avoid any
-- driver-level placeholder ambiguity.
UPDATE users
SET privacy_settings = privacy_settings - 'attendance'
WHERE privacy_settings IS NOT NULL
  AND jsonb_exists(privacy_settings, 'attendance');

ALTER TABLE users ALTER COLUMN privacy_settings SET DEFAULT '{"contributions":"visible","saved_shows":"hidden","following":"visible","collections":"visible","last_active":"visible","profile_sections":"visible"}';

COMMENT ON TABLE user_bookmarks IS 'Generic user-entity relationship table supporting save, follow, bookmark actions across all entity types';
COMMENT ON COLUMN user_bookmarks.action IS 'Action type: save, follow, bookmark';
