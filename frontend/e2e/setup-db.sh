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
# Block until the migrate container exits; propagates its exit code.
# (docker compose wait is available since Compose v2.23)
if ! docker compose -p e2e -f docker-compose.e2e.yml wait migrate; then
  echo "ERROR: Migrations failed. Container logs:"
  docker compose -p e2e -f docker-compose.e2e.yml logs migrate
  exit 1
fi
echo "    Migrations completed successfully."

echo "==> Seeding venues and artists..."
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
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

echo "==> Seeding labels, releases, and linking them..."
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
-- Labels for the discovery loop: Show -> Artist -> Release -> Label -> label mates
INSERT INTO labels (name, slug, status, created_at, updated_at)
VALUES
  ('Run For Cover Records', 'run-for-cover-records', 'active', NOW(), NOW()),
  ('Loma Vista Recordings', 'loma-vista-recordings', 'active', NOW(), NOW()),
  ('Topshelf Records', 'topshelf-records', 'active', NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Releases linked to seeded artists
INSERT INTO releases (title, slug, release_type, release_year, created_at, updated_at)
VALUES
  ('Futures', 'futures', 'lp', 2004, NOW(), NOW()),
  ('Clarity', 'clarity', 'lp', 1999, NOW(), NOW()),
  ('Feast of the Mau Mau', 'feast-of-the-mau-mau', 'lp', 1998, NOW(), NOW()),
  ('Knife Man', 'knife-man', 'lp', 2011, NOW(), NOW()),
  ('Sundressed EP', 'sundressed-ep', 'ep', 2014, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Link artists to releases (artist_releases junction)
DO $$
DECLARE
  a_jimmy INTEGER;
  a_calexico INTEGER;
  a_ajj INTEGER;
  a_sundressed INTEGER;
  r_futures INTEGER;
  r_clarity INTEGER;
  r_feast INTEGER;
  r_knife INTEGER;
  r_sundressed INTEGER;
BEGIN
  SELECT id INTO a_jimmy FROM artists WHERE slug = 'jimmy-eat-world';
  SELECT id INTO a_calexico FROM artists WHERE slug = 'calexico';
  SELECT id INTO a_ajj FROM artists WHERE slug = 'ajj';
  SELECT id INTO a_sundressed FROM artists WHERE slug = 'sundressed';
  SELECT id INTO r_futures FROM releases WHERE slug = 'futures';
  SELECT id INTO r_clarity FROM releases WHERE slug = 'clarity';
  SELECT id INTO r_feast FROM releases WHERE slug = 'feast-of-the-mau-mau';
  SELECT id INTO r_knife FROM releases WHERE slug = 'knife-man';
  SELECT id INTO r_sundressed FROM releases WHERE slug = 'sundressed-ep';

  IF a_jimmy IS NOT NULL AND r_futures IS NOT NULL THEN
    INSERT INTO artist_releases (artist_id, release_id, role, position)
    VALUES (a_jimmy, r_futures, 'main', 0) ON CONFLICT DO NOTHING;
  END IF;
  IF a_jimmy IS NOT NULL AND r_clarity IS NOT NULL THEN
    INSERT INTO artist_releases (artist_id, release_id, role, position)
    VALUES (a_jimmy, r_clarity, 'main', 0) ON CONFLICT DO NOTHING;
  END IF;
  IF a_calexico IS NOT NULL AND r_feast IS NOT NULL THEN
    INSERT INTO artist_releases (artist_id, release_id, role, position)
    VALUES (a_calexico, r_feast, 'main', 0) ON CONFLICT DO NOTHING;
  END IF;
  IF a_ajj IS NOT NULL AND r_knife IS NOT NULL THEN
    INSERT INTO artist_releases (artist_id, release_id, role, position)
    VALUES (a_ajj, r_knife, 'main', 0) ON CONFLICT DO NOTHING;
  END IF;
  IF a_sundressed IS NOT NULL AND r_sundressed IS NOT NULL THEN
    INSERT INTO artist_releases (artist_id, release_id, role, position)
    VALUES (a_sundressed, r_sundressed, 'main', 0) ON CONFLICT DO NOTHING;
  END IF;
END $$;

-- Link releases to labels (release_labels junction) — completes the discovery loop
DO $$
DECLARE
  l_runforcover INTEGER;
  l_lomavista INTEGER;
  l_topshelf INTEGER;
  r_futures INTEGER;
  r_clarity INTEGER;
  r_knife INTEGER;
  r_sundressed INTEGER;
BEGIN
  SELECT id INTO l_runforcover FROM labels WHERE slug = 'run-for-cover-records';
  SELECT id INTO l_lomavista FROM labels WHERE slug = 'loma-vista-recordings';
  SELECT id INTO l_topshelf FROM labels WHERE slug = 'topshelf-records';
  SELECT id INTO r_futures FROM releases WHERE slug = 'futures';
  SELECT id INTO r_clarity FROM releases WHERE slug = 'clarity';
  SELECT id INTO r_knife FROM releases WHERE slug = 'knife-man';
  SELECT id INTO r_sundressed FROM releases WHERE slug = 'sundressed-ep';

  -- Jimmy Eat World releases on various labels
  IF l_runforcover IS NOT NULL AND r_futures IS NOT NULL THEN
    INSERT INTO release_labels (release_id, label_id)
    VALUES (r_futures, l_runforcover) ON CONFLICT DO NOTHING;
  END IF;
  IF l_runforcover IS NOT NULL AND r_clarity IS NOT NULL THEN
    INSERT INTO release_labels (release_id, label_id)
    VALUES (r_clarity, l_runforcover) ON CONFLICT DO NOTHING;
  END IF;
  -- AJJ on topshelf
  IF l_topshelf IS NOT NULL AND r_knife IS NOT NULL THEN
    INSERT INTO release_labels (release_id, label_id)
    VALUES (r_knife, l_topshelf) ON CONFLICT DO NOTHING;
  END IF;
  -- Sundressed on topshelf
  IF l_topshelf IS NOT NULL AND r_sundressed IS NOT NULL THEN
    INSERT INTO release_labels (release_id, label_id)
    VALUES (r_sundressed, l_topshelf) ON CONFLICT DO NOTHING;
  END IF;
END $$;

-- Link artists to labels (artist_labels junction)
DO $$
DECLARE
  a_jimmy INTEGER;
  a_ajj INTEGER;
  a_sundressed INTEGER;
  l_runforcover INTEGER;
  l_topshelf INTEGER;
BEGIN
  SELECT id INTO a_jimmy FROM artists WHERE slug = 'jimmy-eat-world';
  SELECT id INTO a_ajj FROM artists WHERE slug = 'ajj';
  SELECT id INTO a_sundressed FROM artists WHERE slug = 'sundressed';
  SELECT id INTO l_runforcover FROM labels WHERE slug = 'run-for-cover-records';
  SELECT id INTO l_topshelf FROM labels WHERE slug = 'topshelf-records';

  IF a_jimmy IS NOT NULL AND l_runforcover IS NOT NULL THEN
    INSERT INTO artist_labels (artist_id, label_id)
    VALUES (a_jimmy, l_runforcover) ON CONFLICT DO NOTHING;
  END IF;
  IF a_ajj IS NOT NULL AND l_topshelf IS NOT NULL THEN
    INSERT INTO artist_labels (artist_id, label_id)
    VALUES (a_ajj, l_topshelf) ON CONFLICT DO NOTHING;
  END IF;
  IF a_sundressed IS NOT NULL AND l_topshelf IS NOT NULL THEN
    INSERT INTO artist_labels (artist_id, label_id)
    VALUES (a_sundressed, l_topshelf) ON CONFLICT DO NOTHING;
  END IF;
END $$;
SQL

echo "==> Inserting future-dated test shows..."
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
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
      -- Rotate 3 cities so the "popular cities" UI (MIN_POPULAR_CITIES=3,
      -- MIN_POPULAR_COUNT=2 in components/filters/CityFilters.tsx) has enough
      -- data to render. Distribution: ~18 Phoenix, ~18 Tucson, ~19 Mesa.
      CASE i % 3
        WHEN 0 THEN 'Tucson'
        WHEN 1 THEN 'Phoenix'
        ELSE 'Mesa'
      END,
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

echo "==> Inserting reserved E2E rows for mutating tests (PSY-430)..."
# Reserved rows that mutating E2E tests target by stable title/slug, so
# parallel workers in different files don't race on the same .first() row.
# Convention: title prefixed with "E2E [<purpose>]", slug pre-set so the
# backfill below skips them and the slug is stable across CI runs.
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
INSERT INTO venues (name, address, city, state, zipcode, verified, created_at, updated_at, slug)
VALUES (
  'E2E [favorite-venue-test]',
  '100 Reserved Way', 'Phoenix', 'AZ', '85001',
  true, NOW(), NOW(), 'e2e-favorite-venue-test'
)
ON CONFLICT DO NOTHING;

DO $$
DECLARE
  v_id INTEGER;
  a_id INTEGER;
  s_id INTEGER;
BEGIN
  SELECT id INTO v_id FROM venues WHERE slug = 'e2e-favorite-venue-test';
  SELECT id INTO a_id FROM artists ORDER BY id LIMIT 1;

  -- Plain INSERTs (no ON CONFLICT): the e2e DB is wiped per-run by Docker,
  -- so duplicates are impossible. The slug unique index is partial
  -- (WHERE slug IS NOT NULL), which makes ON CONFLICT (slug) awkward.

  -- collection.spec.ts "shows saved show after saving one"
  INSERT INTO shows (title, event_date, city, state, status, slug, created_at, updated_at)
  VALUES (
    'E2E [collection-saved-show]',
    NOW() + INTERVAL '1 hour',
    'Phoenix', 'AZ', 'approved',
    'e2e-collection-saved-show',
    NOW(), NOW()
  )
  RETURNING id INTO s_id;
  INSERT INTO show_venues (show_id, venue_id) VALUES (s_id, v_id);
  INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (s_id, a_id, 0, 'headliner');

  -- save-show.spec.ts (both mutating tests)
  INSERT INTO shows (title, event_date, city, state, status, slug, created_at, updated_at)
  VALUES (
    'E2E [save-show-test]',
    NOW() + INTERVAL '2 hours',
    'Phoenix', 'AZ', 'approved',
    'e2e-save-show-test',
    NOW(), NOW()
  )
  RETURNING id INTO s_id;
  INSERT INTO show_venues (show_id, venue_id) VALUES (s_id, v_id);
  INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (s_id, a_id, 0, 'headliner');

  -- show-list-actions.spec.ts "toggle save state from list cards"
  INSERT INTO shows (title, event_date, city, state, status, slug, created_at, updated_at)
  VALUES (
    'E2E [list-actions-test]',
    NOW() + INTERVAL '3 hours',
    'Phoenix', 'AZ', 'approved',
    'e2e-list-actions-test',
    NOW(), NOW()
  )
  RETURNING id INTO s_id;
  INSERT INTO show_venues (show_id, venue_id) VALUES (s_id, v_id);
  INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (s_id, a_id, 0, 'headliner');
END $$;
SQL

echo "==> Verifying seeded venues (public API requires verified=true)..."
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
UPDATE venues SET verified = true WHERE verified = false;
SQL

echo "==> Backfilling slugs for any records missing them..."
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
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

psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<SQL
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
INSERT INTO user_preferences (user_id, notification_email, notification_push, show_reminders, theme, timezone, language, created_at, updated_at)
SELECT id, true, false, false, 'system', 'America/Phoenix', 'en', NOW(), NOW()
FROM users WHERE email IN ('e2e-user@test.local', 'e2e-admin@test.local', 'e2e-unverified@test.local')
ON CONFLICT (user_id) DO NOTHING;
SQL

echo "==> Seeding radio stations and shows..."
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
-- Radio stations: KEXP, WFMU, NTS
INSERT INTO radio_stations (name, slug, description, city, state, country, timezone, stream_url, website, donation_url, broadcast_type, frequency_mhz, playlist_source, is_active, created_at, updated_at)
VALUES
  ('KEXP', 'kexp', 'Listener-supported non-commercial radio in Seattle, championing independent and emerging artists.', 'Seattle', 'WA', 'US', 'America/Los_Angeles', 'https://kexp.streamguys1.com/kexp160.aac', 'https://www.kexp.org', 'https://www.kexp.org/donate/', 'both', 90.3, 'kexp_api', true, NOW(), NOW()),
  ('WFMU', 'wfmu', 'The longest-running freeform radio station in the United States, broadcasting from Jersey City.', 'Jersey City', 'NJ', 'US', 'America/New_York', 'https://stream0.wfmu.org/freeform-128k', 'https://wfmu.org', 'https://wfmu.org/marathon/', 'both', 91.1, 'wfmu_scrape', true, NOW(), NOW()),
  ('NTS Radio', 'nts-radio', 'Online radio station based in London, broadcasting 24/7 across two channels.', 'London', '', 'GB', 'Europe/London', 'https://stream-relay-geo.ntslive.net/stream', 'https://www.nts.live', 'https://www.nts.live/supporters', 'internet', NULL, 'nts_api', true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Radio shows with archive URLs
DO $$
DECLARE
  kexp_id INTEGER;
  wfmu_id INTEGER;
  nts_id INTEGER;
BEGIN
  SELECT id INTO kexp_id FROM radio_stations WHERE slug = 'kexp';
  SELECT id INTO wfmu_id FROM radio_stations WHERE slug = 'wfmu';
  SELECT id INTO nts_id FROM radio_stations WHERE slug = 'nts-radio';

  IF kexp_id IS NOT NULL THEN
    INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active, created_at, updated_at)
    VALUES
      (kexp_id, 'The Morning Show', 'the-morning-show', 'John Richards', 'KEXP flagship morning program.', 'Weekdays 6-10 AM PT', 'https://www.kexp.org/shows/the-morning-show/', '1', true, NOW(), NOW()),
      (kexp_id, 'The Midday Show', 'the-midday-show', 'Cheryl Waters', 'Mid-day new music discoveries and deep cuts.', 'Weekdays 10 AM-2 PM PT', 'https://www.kexp.org/shows/the-midday-show/', '2', true, NOW(), NOW()),
      (kexp_id, 'Audioasis', 'audioasis', 'Kennady Quille', 'Northwest music show.', 'Saturdays 6-9 PM PT', 'https://www.kexp.org/shows/Audioasis/', '4', true, NOW(), NOW())
    ON CONFLICT DO NOTHING;
  END IF;

  IF wfmu_id IS NOT NULL THEN
    INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active, created_at, updated_at)
    VALUES
      (wfmu_id, 'Trouble', 'trouble-wfmu', 'Doug Schulkind', 'Eclectic freeform with soul, jazz, and international.', 'Wednesdays 10 AM-1 PM ET', 'https://wfmu.org/playlists/DS', 'DS', true, NOW(), NOW()),
      (wfmu_id, 'The Best Show', 'the-best-show-wfmu', 'Tom Scharpling', 'Comedy and music variety show.', 'Tuesdays 9 PM-12 AM ET', 'https://wfmu.org/playlists/TS', 'TS', true, NOW(), NOW())
    ON CONFLICT DO NOTHING;
  END IF;

  IF nts_id IS NOT NULL THEN
    INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active, created_at, updated_at)
    VALUES
      (nts_id, 'Floating Points', 'floating-points-nts', 'Floating Points', 'Jazz, electronic, ambient, and world music.', 'Monthly', 'https://www.nts.live/shows/floating-points', 'floating-points', true, NOW(), NOW()),
      (nts_id, 'Charlie Bones', 'charlie-bones-nts', 'Charlie Bones', 'Eclectic morning show with jazz, soul, funk.', 'Weekdays 10 AM-1 PM GMT', 'https://www.nts.live/shows/charlie-bones', 'charlie-bones', true, NOW(), NOW())
    ON CONFLICT DO NOTHING;
  END IF;
END $$;
SQL

echo "==> Inserting admin workflow test data..."
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
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
