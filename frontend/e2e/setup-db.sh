#!/usr/bin/env bash
#
# Seeds the E2E test database with venues, artists, past shows,
# then inserts future-dated shows and test user accounts.
#
# Must be run from the backend/ directory.
#
set -euo pipefail

E2E_DB_URL="postgres://e2euser:e2epassword@localhost:5433/e2edb?sslmode=disable"

echo "==> Waiting for migrate service to finish..."
# Wait up to 60s for the migrate container to exit successfully
for i in $(seq 1 30); do
  STATUS=$(docker compose -f docker-compose.e2e.yml ps migrate --format json 2>/dev/null | python3 -c "import sys,json; data=json.load(sys.stdin); print(data['State'] if isinstance(data, dict) else data[0]['State'])" 2>/dev/null || echo "unknown")
  if [ "$STATUS" = "exited" ]; then
    EXIT_CODE=$(docker compose -f docker-compose.e2e.yml ps migrate --format json 2>/dev/null | python3 -c "import sys,json; data=json.load(sys.stdin); print(data['ExitCode'] if isinstance(data, dict) else data[0]['ExitCode'])" 2>/dev/null || echo "1")
    if [ "$EXIT_CODE" = "0" ]; then
      echo "    Migrations completed successfully."
      break
    else
      echo "ERROR: Migrate exited with code $EXIT_CODE"
      docker compose -f docker-compose.e2e.yml logs migrate
      exit 1
    fi
  fi
  sleep 2
done

echo "==> Running seed command..."
DATABASE_URL="$E2E_DB_URL" go run ./cmd/seed

echo "==> Inserting future-dated test shows..."
psql "$E2E_DB_URL" <<'SQL'
-- Insert 55 future-dated approved shows (enough to trigger pagination at limit=50).
-- Uses venue/artist IDs from the seed data, cycling through them.
DO $$
DECLARE
  v_ids INTEGER[];
  a_ids INTEGER[];
  s_id INTEGER;
  i INTEGER;
BEGIN
  -- Collect venue IDs
  SELECT array_agg(id ORDER BY id) INTO v_ids FROM (SELECT id FROM venues LIMIT 5) sub;
  -- Collect artist IDs
  SELECT array_agg(id ORDER BY id) INTO a_ids FROM (SELECT id FROM artists LIMIT 10) sub;

  FOR i IN 1..55 LOOP
    INSERT INTO shows (title, event_date, city, state, status, created_at, updated_at)
    VALUES (
      'E2E Test Show ' || i,
      NOW() + (i || ' days')::INTERVAL,
      CASE WHEN i % 3 = 0 THEN 'Tucson' ELSE 'Phoenix' END,
      'AZ',
      'approved',
      NOW(), NOW()
    )
    RETURNING id INTO s_id;

    INSERT INTO show_venues (show_id, venue_id)
    VALUES (s_id, v_ids[1 + (i % array_length(v_ids, 1))]);

    INSERT INTO show_artists (show_id, artist_id, position, set_type)
    VALUES (s_id, a_ids[1 + (i % array_length(a_ids, 1))], 0, 'headliner');

    -- Add an opener to every other show
    IF i % 2 = 0 THEN
      INSERT INTO show_artists (show_id, artist_id, position, set_type)
      VALUES (s_id, a_ids[1 + ((i + 1) % array_length(a_ids, 1))], 1, 'opener');
    END IF;
  END LOOP;
END $$;
SQL

echo "==> Verifying seeded venues (public API requires verified=true)..."
psql "$E2E_DB_URL" <<'SQL'
UPDATE venues SET verified = true WHERE verified = false;
SQL

echo "==> Backfilling slugs for any records missing them..."
psql "$E2E_DB_URL" <<'SQL'
-- Backfill show slugs (the SQL-inserted E2E shows don't have slugs)
UPDATE shows
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(
            CONCAT(TO_CHAR(event_date, 'YYYY-MM-DD'), '-', title),
            '[^a-zA-Z0-9\s-]', '', 'g'
        ),
        '\s+', '-', 'g'
    )
)
WHERE slug IS NULL;

-- Deduplicate any colliding show slugs by appending the ID
UPDATE shows s1
SET slug = s1.slug || '-' || s1.id
WHERE EXISTS (
    SELECT 1 FROM shows s2
    WHERE s2.slug = s1.slug AND s2.id < s1.id
);
SQL

echo "==> Inserting test users..."
# Pre-computed bcrypt hash for "e2e-test-password-123" (cost 10)
BCRYPT_HASH='$2a$10$h7GdGcX7SxMFQCohXdTQnuVkygj7RPCcPhPrPMgHkWr50w.Fv0XoW'

psql "$E2E_DB_URL" <<SQL
-- Regular test user
INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, created_at, updated_at)
VALUES ('e2e-user@test.local', '${BCRYPT_HASH}', 'Test', 'User', true, false, true, NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- Admin test user
INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, created_at, updated_at)
VALUES ('e2e-admin@test.local', '${BCRYPT_HASH}', 'Test', 'Admin', true, true, true, NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- Unverified test user (for email verification tests)
INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, created_at, updated_at)
VALUES ('e2e-unverified@test.local', '${BCRYPT_HASH}', 'Test', 'Unverified', true, false, false, NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- Create user_preferences for all test users
INSERT INTO user_preferences (user_id, notification_email, notification_push, theme, timezone, language, created_at, updated_at)
SELECT id, true, false, 'system', 'America/Phoenix', 'en', NOW(), NOW()
FROM users WHERE email IN ('e2e-user@test.local', 'e2e-admin@test.local', 'e2e-unverified@test.local')
ON CONFLICT (user_id) DO NOTHING;
SQL

echo "==> E2E database setup complete!"
