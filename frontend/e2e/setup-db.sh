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

echo "==> Seeding venues and artists..."
psql "$E2E_DB_URL" <<'SQL'
-- Minimal set of venues for E2E tests
INSERT INTO venues (name, address, city, state, zipcode, verified, created_at, updated_at, slug)
VALUES
  ('The Rebel Lounge', '2303 E Indian School Rd', 'Phoenix', 'AZ', '85016', true, NOW(), NOW(), 'the-rebel-lounge-phoenix-az'),
  ('Crescent Ballroom', '308 N 2nd Ave', 'Phoenix', 'AZ', '85003', true, NOW(), NOW(), 'crescent-ballroom-phoenix-az'),
  ('Valley Bar', '130 N Central Ave', 'Phoenix', 'AZ', '85004', true, NOW(), NOW(), 'valley-bar-phoenix-az'),
  ('Club Congress', '311 E Congress St', 'Tucson', 'AZ', '85701', true, NOW(), NOW(), 'club-congress-tucson-az'),
  ('191 Toole', '191 E Toole Ave', 'Tucson', 'AZ', '85701', true, NOW(), NOW(), '191-toole-tucson-az'),
  ('Hotel Congress Plaza', '311 E Congress St', 'Tucson', 'AZ', '85701', true, NOW(), NOW(), 'hotel-congress-plaza-tucson-az')
ON CONFLICT DO NOTHING;

-- Minimal set of artists for E2E tests
INSERT INTO artists (name, created_at, updated_at, slug)
VALUES
  ('Calexico', NOW(), NOW(), 'calexico'),
  ('Jimmy Eat World', NOW(), NOW(), 'jimmy-eat-world'),
  ('The Maine', NOW(), NOW(), 'the-maine'),
  ('AJJ', NOW(), NOW(), 'ajj'),
  ('Playboy Manbaby', NOW(), NOW(), 'playboy-manbaby'),
  ('Doll Skin', NOW(), NOW(), 'doll-skin'),
  ('The Format', NOW(), NOW(), 'the-format'),
  ('Dear and the Headlights', NOW(), NOW(), 'dear-and-the-headlights'),
  ('Sundressed', NOW(), NOW(), 'sundressed'),
  ('ROAR', NOW(), NOW(), 'roar')
ON CONFLICT DO NOTHING;
SQL

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

echo "==> Inserting admin workflow test data..."
psql "$E2E_DB_URL" <<'SQL'
-- Admin workflow seed data: pending shows, unverified venue, pending venue edits
DO $$
DECLARE
  test_user_id INTEGER;
  v_id INTEGER;
  a_id INTEGER;
  s_id INTEGER;
  venue1_id INTEGER;
  venue2_id INTEGER;
BEGIN
  -- Get test user ID
  SELECT id INTO test_user_id FROM users WHERE email = 'e2e-user@test.local';

  -- Get an existing venue and artist for pending shows
  SELECT id INTO v_id FROM venues LIMIT 1;
  SELECT id INTO a_id FROM artists LIMIT 1;

  -- 1) Two pending shows for approve/reject tests
  INSERT INTO shows (title, event_date, city, state, status, source, submitted_by, created_at, updated_at, slug)
  VALUES (
    'E2E Pending Show Approve',
    NOW() + INTERVAL '90 days',
    'Phoenix', 'AZ', 'pending', 'user', test_user_id,
    NOW(), NOW(),
    'e2e-pending-show-approve'
  )
  RETURNING id INTO s_id;
  INSERT INTO show_venues (show_id, venue_id) VALUES (s_id, v_id);
  INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (s_id, a_id, 0, 'headliner');

  INSERT INTO shows (title, event_date, city, state, status, source, submitted_by, created_at, updated_at, slug)
  VALUES (
    'E2E Pending Show Reject',
    NOW() + INTERVAL '91 days',
    'Phoenix', 'AZ', 'pending', 'user', test_user_id,
    NOW(), NOW(),
    'e2e-pending-show-reject'
  )
  RETURNING id INTO s_id;
  INSERT INTO show_venues (show_id, venue_id) VALUES (s_id, v_id);
  INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (s_id, a_id, 0, 'headliner');

  -- 2) Unverified venue
  INSERT INTO venues (name, address, city, state, zipcode, verified, created_at, updated_at, slug)
  VALUES (
    'E2E Unverified Venue',
    '999 Test Street', 'Phoenix', 'AZ', '85001',
    false, NOW(), NOW(), 'e2e-unverified-venue'
  );

  -- 3) Two pending venue edits against existing venues
  SELECT id INTO venue1_id FROM venues WHERE verified = true ORDER BY id LIMIT 1;
  SELECT id INTO venue2_id FROM venues WHERE verified = true AND id != venue1_id ORDER BY id LIMIT 1;

  -- Edit 1: propose address + website change (for approve test)
  INSERT INTO pending_venue_edits (venue_id, submitted_by, address, website, status, created_at, updated_at)
  VALUES (venue1_id, test_user_id, '123 Updated Address', 'https://updated-venue.example.com', 'pending', NOW(), NOW());

  -- Edit 2: propose name change (for reject test)
  INSERT INTO pending_venue_edits (venue_id, submitted_by, name, status, created_at, updated_at)
  VALUES (venue2_id, test_user_id, 'Renamed Venue E2E', 'pending', NOW(), NOW());

  -- 4) Approved show submitted by test user (for "my submissions" test)
  INSERT INTO shows (title, event_date, city, state, status, source, submitted_by, created_at, updated_at, slug)
  VALUES (
    'E2E My Submitted Show',
    NOW() + INTERVAL '85 days',
    'Phoenix', 'AZ', 'approved', 'user', test_user_id,
    NOW(), NOW(),
    'e2e-my-submitted-show'
  )
  RETURNING id INTO s_id;
  INSERT INTO show_venues (show_id, venue_id) VALUES (s_id, v_id);
  INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (s_id, a_id, 0, 'headliner');

END $$;
SQL

echo "==> E2E database setup complete!"
