#!/usr/bin/env bash
#
# Seeds the E2E test database with venues, artists, past shows,
# then inserts future-dated shows and test user accounts.
#
# Must be run from the backend/ directory.
#
set -euo pipefail

E2E_DB_URL="${DATABASE_URL:-postgres://e2euser:e2epassword@localhost:5433/e2edb?sslmode=disable}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-e2e}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.e2e.yml}"

echo "==> Waiting for migrate service to finish..."
# Block until the migrate container exits; propagates its exit code.
#
# `docker compose wait` is unreliable here: it returns "no containers for
# project" with exit 1 once the container has already exited, which happens
# when the parent already used `up -d --wait` (PSY-624 dispatch path) and
# the migrate one-shot finished before we polled. Inspect ps state directly.
#
# Output format is one JSON object per service (NDJSON) — `--format json
# <service>` filters to a single line. Tested against Docker Compose v5.0.1.
wait_migrate_done() {
  local timeout_sec="${1:-60}" start row state exit_code
  start=$(date +%s)
  while true; do
    row=$(docker compose -p "$COMPOSE_PROJECT" -f "$COMPOSE_FILE" \
      ps -a --format json migrate 2>/dev/null) || row=""
    if [ -n "$row" ]; then
      state=$(echo "$row" | jq -r '.State')
      exit_code=$(echo "$row" | jq -r '.ExitCode')
      case "$state" in
        exited)
          if [ "$exit_code" = "0" ]; then
            return 0
          fi
          echo "ERROR: migrate exited with code $exit_code"
          return 1
          ;;
        running|created|restarting)
          ;;
        *)
          echo "WARN: migrate in unexpected state '$state'"
          ;;
      esac
    fi
    if [ "$(($(date +%s) - start))" -ge "$timeout_sec" ]; then
      echo "ERROR: timeout waiting for migrate to finish"
      return 1
    fi
    sleep 0.5
  done
}

if ! wait_migrate_done 60; then
  echo "Container logs:"
  docker compose -p "$COMPOSE_PROJECT" -f "$COMPOSE_FILE" logs migrate
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

-- Festival fixture (PSY-904): the festivals table is a real DB-stored entity
-- (migration 000039), unlike blog/dj-sets (local MDX) and scenes (derived from
-- venue/show/artist aggregates). The /festivals/[slug] detail page fetches
-- /festivals/<slug> (backend GetFestivalHandler falls back to GetFestivalBySlug
-- for a non-numeric path), so a deterministic slug here gives content-detail.spec
-- a stable festival URL to assert H1 + canonical + JSON-LD against. NOT NULL
-- columns per the migration: name, slug, series_slug, edition_year, start_date,
-- end_date. City/state = Phoenix, AZ matches the rest of the seed (and increments
-- the phoenix-az scene's festival_count — a harmless side benefit, not relied on).
INSERT INTO festivals (
  name, slug, series_slug, edition_year,
  description, city, state, country,
  start_date, end_date, status,
  created_at, updated_at
)
VALUES (
  'E2E Test Fest 2026', 'e2e-test-fest-2026', 'e2e-test-fest', 2026,
  'Seeded festival fixture for content-detail E2E coverage (PSY-904).',
  'Phoenix', 'AZ', 'US',
  '2026-09-18', '2026-09-20', 'confirmed',
  NOW(), NOW()
)
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
VALUES
  (
    'E2E [reserved-venue]',
    '100 Reserved Way', 'Phoenix', 'AZ', '85001',
    true, NOW(), NOW(), 'e2e-reserved-venue'
  ),
  -- PSY-456: one reserved venue per comments test so the 60s per-entity
  -- comment cooldown (user_id + entity_type + entity_id) can't collide
  -- across the create/vote/reply tests within this spec.
  (
    'E2E [comment-create]',
    '200 Reserved Way', 'Phoenix', 'AZ', '85001',
    true, NOW(), NOW(), 'e2e-comment-create'
  ),
  (
    'E2E [comment-vote]',
    '201 Reserved Way', 'Phoenix', 'AZ', '85001',
    true, NOW(), NOW(), 'e2e-comment-vote'
  ),
  (
    'E2E [comment-reply]',
    '202 Reserved Way', 'Phoenix', 'AZ', '85001',
    true, NOW(), NOW(), 'e2e-comment-reply'
  )
ON CONFLICT DO NOTHING;

-- PSY-457: reserved artist for follow-and-attendance.spec.ts. Dedicated row
-- (not Calexico) so cross-worker follow tests don't race on a shared artist.
INSERT INTO artists (name, slug, created_at, updated_at)
VALUES (
  'E2E [follow-test]',
  'e2e-follow-test',
  NOW(), NOW()
)
ON CONFLICT DO NOTHING;

DO $$
DECLARE
  v_id INTEGER;
  a_id INTEGER;
  s_id INTEGER;
BEGIN
  SELECT id INTO v_id FROM venues WHERE slug = 'e2e-reserved-venue';
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

  -- add-to-collection.spec.ts "adds a show to a collection from the detail page"
  -- (PSY-455). Dedicated reserved show so we don't race with save-show or
  -- collection.spec.ts which both mutate e2e-collection-saved-show.
  INSERT INTO shows (title, event_date, city, state, status, slug, created_at, updated_at)
  VALUES (
    'E2E [add-to-collection-test]',
    NOW() + INTERVAL '4 hours',
    'Phoenix', 'AZ', 'approved',
    'e2e-add-to-collection-test',
    NOW(), NOW()
  )
  RETURNING id INTO s_id;
  INSERT INTO show_venues (show_id, venue_id) VALUES (s_id, v_id);
  INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (s_id, a_id, 0, 'headliner');

  -- PSY-457: follow-and-attendance.spec.ts "mark going/interested"
  -- Dedicated show so parallel workers don't race on e2e-collection-saved-show.
  INSERT INTO shows (title, event_date, city, state, status, slug, created_at, updated_at)
  VALUES (
    'E2E [attendance-test]',
    NOW() + INTERVAL '5 hours',
    'Phoenix', 'AZ', 'approved',
    'e2e-attendance-test',
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
-- Regular test users: one per Playwright worker to avoid mutation races (PSY-431).
-- Worker 0 uses e2e-user@test.local (legacy); workers 1-4 use numbered variants.
-- Seeded count (5) >= max local worker count; CI uses 3 workers.
--
-- PSY-456: user_tier = 'contributor' so new comments publish as 'visible'
-- immediately (new_user tier -> 'pending_review'). Without this, the
-- comments E2E spec would have to assert on pending_review state or wait
-- for moderation.
INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, user_tier, created_at, updated_at)
VALUES
  ('e2e-user@test.local',   '${BCRYPT_HASH}', 'Test', 'User 0', true, false, true, 'contributor', NOW(), NOW()),
  ('e2e-user-1@test.local', '${BCRYPT_HASH}', 'Test', 'User 1', true, false, true, 'contributor', NOW(), NOW()),
  ('e2e-user-2@test.local', '${BCRYPT_HASH}', 'Test', 'User 2', true, false, true, 'contributor', NOW(), NOW()),
  ('e2e-user-3@test.local', '${BCRYPT_HASH}', 'Test', 'User 3', true, false, true, 'contributor', NOW(), NOW()),
  ('e2e-user-4@test.local', '${BCRYPT_HASH}', 'Test', 'User 4', true, false, true, 'contributor', NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- Admin test user (single; admin tests are rare and low-race-risk)
INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, created_at, updated_at)
VALUES ('e2e-admin@test.local', '${BCRYPT_HASH}', 'Test', 'Admin', true, true, true, NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- Unverified test user (for email verification tests)
INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, created_at, updated_at)
VALUES ('e2e-unverified@test.local', '${BCRYPT_HASH}', 'Test', 'Unverified', true, false, false, NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- Soft-deleted, still-recoverable test user (PSY-719: account-recovery
-- completion flow). is_active=false + deleted_at=NOW() puts the account inside
-- the 30-day AccountRecoveryGracePeriod, so ConfirmAccountRecoveryHandler
-- restores it and logs in. Dedicated user (not a worker login) so restoring it
-- mid-suite can't disturb the parallel worker auth state.
INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, deleted_at, deletion_reason, created_at, updated_at)
VALUES ('e2e-recovery@test.local', '${BCRYPT_HASH}', 'Test', 'Recovery', false, false, true, NOW(), 'e2e recovery fixture', NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- OAuth login fixture (PSY-914). The faux "google" provider
-- (ENABLE_OAUTH_TEST_PROVIDER=1, backend/internal/auth/oauth_test_provider.go)
-- always returns email e2e-oauth@test.local. Pre-seeding that user means the
-- first faux login resolves to an EXISTING user (linkOAuthAccount = a login),
-- NOT a new signup — so oauth-google.spec.ts never hits the terms/consent flow.
-- Dedicated user (not a worker login) so linking an oauth_accounts row to it
-- can't disturb the parallel worker auth state. No password login is expected
-- (OAuth only); the shared hash is set just for parity with the other fixtures.
INSERT INTO users (email, password_hash, first_name, last_name, is_active, is_admin, email_verified, user_tier, created_at, updated_at)
VALUES ('e2e-oauth@test.local', '${BCRYPT_HASH}', 'Test', 'OAuth', true, false, true, 'contributor', NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- Create user_preferences for all seeded test users (regular worker users + admin + unverified + recovery + oauth)
INSERT INTO user_preferences (user_id, notification_email, notification_push, show_reminders, theme, timezone, language, created_at, updated_at)
SELECT id, true, false, false, 'system', 'America/Phoenix', 'en', NOW(), NOW()
FROM users
WHERE email LIKE 'e2e-user%@test.local'
   OR email IN ('e2e-admin@test.local', 'e2e-unverified@test.local', 'e2e-recovery@test.local', 'e2e-oauth@test.local')
ON CONFLICT (user_id) DO NOTHING;
SQL

echo "==> Seeding representative tags + entity_tags so facet panels render non-empty (PSY-1010)..."
# The dev Go seed (cmd/seed -> exemplars.go) tags its *-exemplar entities, but
# this E2E/dispatch-stack seed shipped with ZERO tags — so every tag-facet
# browse page (/shows /artists /releases /venues /labels /festivals) rendered an
# EMPTY facet panel by default and tag-work agents had to hand-seed to repro.
#
# Seed a SMALL, representative vocabulary and apply the SAME few tags across
# MULTIPLE entity types so cross-entity facets are visible. The facet panel
# (frontend/features/tags/components/TagFacetPanel.tsx) HIDES zero-count chips
# per entity type and renders nothing when every chip in a category is zero, so
# each browse page needs at least one tag with a NON-ZERO count for its type.
#
# Transitive types (PSY-499): /shows and /festivals match a tag TRANSITIVELY via
# their billed artists — there are no direct show/festival tags. So we tag the
# ARTISTS that are billed on the seeded future shows (the 55-show loop above
# bills the first 10 artists) and add a lineup to the seeded festival, rather
# than tagging shows/festivals directly (a direct tag would never surface in the
# transitive facet count).
#
# Tags are authored by the admin user (entity_tags.added_by_user_id is NOT NULL),
# which is seeded in the users block above. Idempotent via ON CONFLICT so a
# dispatch stack that re-runs setup-db.sh neither duplicates nor errors.
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
-- Small representative tag vocabulary. Genre tags drive cross-entity discovery;
-- one locale tag ('phoenix') populates a second facet-category group so the
-- panel shows more than the single 'genre' column. is_official=true mirrors
-- how the dev exemplar seed creates its tags.
INSERT INTO tags (name, slug, category, is_official, created_at, updated_at)
VALUES
  ('Post-Punk', 'post-punk', 'genre',  true, NOW(), NOW()),
  ('Shoegaze',  'shoegaze',  'genre',  true, NOW(), NOW()),
  ('Noise',     'noise',     'genre',  true, NOW(), NOW()),
  ('Ambient',   'ambient',   'genre',  true, NOW(), NOW()),
  ('Emo',       'emo',       'genre',  true, NOW(), NOW()),
  ('Phoenix',   'phoenix',   'locale', true, NOW(), NOW())
ON CONFLICT (slug) DO NOTHING;

DO $$
DECLARE
  tagger_id INTEGER;
  fest_id INTEGER;
  -- (entity_type, slug, tag_slug) triples: which tag to apply to which entity.
  -- Artists here are billed on the 55-show loop's bill (first 10 artists) so
  -- tagging them makes the transitive /shows facet non-empty AND the direct
  -- /artists facet non-empty. The same few tags repeat across releases /
  -- venues / labels so a single tag (e.g. 'emo') spans several browse pages.
  app RECORD;
  ent_id INTEGER;
  -- Artists added to the festival lineup so the transitive /festivals facet is
  -- non-empty (the seeded festival had no lineup). Reuse already-tagged artists.
  lineup RECORD;
  pos INTEGER := 0;
BEGIN
  SELECT id INTO tagger_id FROM users WHERE email = 'e2e-admin@test.local';
  IF tagger_id IS NULL THEN
    RAISE NOTICE 'PSY-1010: admin user not found; skipping tag seed';
    RETURN;
  END IF;

  -- Direct entity_tags applications across artist / release / venue / label.
  FOR app IN
    SELECT * FROM (VALUES
      -- Artists (also drive transitive /shows via the bill).
      ('artist',  'jimmy-eat-world', 'emo'),
      ('artist',  'jimmy-eat-world', 'shoegaze'),
      ('artist',  'the-format',      'emo'),
      ('artist',  'calexico',        'ambient'),
      ('artist',  'ajj',             'post-punk'),
      ('artist',  'sundressed',      'emo'),
      ('artist',  'the-maine',       'shoegaze'),
      -- Releases (direct /releases facet).
      ('release', 'futures',              'emo'),
      ('release', 'clarity',              'emo'),
      ('release', 'knife-man',            'post-punk'),
      ('release', 'feast-of-the-mau-mau', 'ambient'),
      ('release', 'sundressed-ep',        'shoegaze'),
      -- Venues (direct /venues facet; 'phoenix' locale tag populates a 2nd
      -- facet category on the venues page).
      ('venue',   'the-rebel-lounge-phoenix-az', 'phoenix'),
      ('venue',   'crescent-ballroom-phoenix-az', 'phoenix'),
      ('venue',   'valley-bar-phoenix-az',        'noise'),
      ('venue',   'club-congress-tucson-az',      'post-punk'),
      -- Labels (direct /labels facet).
      ('label',   'run-for-cover-records', 'emo'),
      ('label',   'topshelf-records',      'post-punk'),
      ('label',   'loma-vista-recordings', 'noise')
    ) AS t(entity_type, entity_slug, tag_slug)
  LOOP
    -- Resolve the entity ID from its slug per table.
    ent_id := NULL;
    IF app.entity_type = 'artist' THEN
      SELECT id INTO ent_id FROM artists WHERE slug = app.entity_slug;
    ELSIF app.entity_type = 'release' THEN
      SELECT id INTO ent_id FROM releases WHERE slug = app.entity_slug;
    ELSIF app.entity_type = 'venue' THEN
      SELECT id INTO ent_id FROM venues WHERE slug = app.entity_slug;
    ELSIF app.entity_type = 'label' THEN
      SELECT id INTO ent_id FROM labels WHERE slug = app.entity_slug;
    END IF;

    IF ent_id IS NOT NULL THEN
      INSERT INTO entity_tags (tag_id, entity_type, entity_id, added_by_user_id, created_at)
      SELECT t.id, app.entity_type, ent_id, tagger_id, NOW()
      FROM tags t WHERE t.slug = app.tag_slug
      ON CONFLICT (tag_id, entity_type, entity_id) DO NOTHING;
    END IF;
  END LOOP;

  -- Festival lineup so the transitive /festivals facet is non-empty. The seeded
  -- 'e2e-test-fest-2026' festival had no festival_artists rows; bill a few of
  -- the tagged artists above so their tags surface transitively.
  SELECT id INTO fest_id FROM festivals WHERE slug = 'e2e-test-fest-2026';
  IF fest_id IS NOT NULL THEN
    FOR lineup IN
      SELECT id, slug FROM artists
      WHERE slug IN ('jimmy-eat-world', 'the-format', 'calexico', 'ajj')
      ORDER BY slug
    LOOP
      INSERT INTO festival_artists (festival_id, artist_id, billing_tier, position, created_at)
      VALUES (fest_id, lineup.id, CASE WHEN pos = 0 THEN 'headliner' ELSE 'mid_card' END, pos, NOW())
      ON CONFLICT (festival_id, artist_id) DO NOTHING;
      pos := pos + 1;
    END LOOP;
  END IF;

  -- Sync each tag's usage_count to its direct entity_tags fan-out (the global
  -- /tags-browse count; per-entity-type facet counts are recomputed live by the
  -- backend regardless). Keeps the seed self-consistent.
  UPDATE tags SET usage_count = sub.cnt
  FROM (
    SELECT tag_id, COUNT(*) AS cnt FROM entity_tags GROUP BY tag_id
  ) sub
  WHERE tags.id = sub.tag_id;
END $$;
SQL

echo "==> Seeding radio stations and shows (generated from backend/internal/seeddata/radio.go)..."
# PSY-414: single source of truth in backend/internal/seeddata/radio.go,
# rendered to SQL by cmd/gen-e2e-seed. cmd/seed (for local dev / stage)
# and this pipe (for E2E) both consume the same Go data, so drift is not
# possible. See docs/runbooks/migrations.md.
go run ./cmd/gen-e2e-seed | psql -v ON_ERROR_STOP=1 "$E2E_DB_URL"

echo "==> Seeding reserved per-worker collections (PSY-455)..."
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
-- One "E2E Worker Collection" per worker-user so add-to-collection.spec.ts
-- can target a collection owned by whichever worker-user picks up the test.
-- Slug = e2e-worker-collection-<user_id>, unique per user per the collections
-- table's UNIQUE(slug) constraint. The e2e DB is wiped per-run by Docker,
-- so no ON CONFLICT is needed.
DO $$
DECLARE
  worker_user RECORD;
BEGIN
  FOR worker_user IN
    SELECT id FROM users WHERE email LIKE 'e2e-user%@test.local'
  LOOP
    INSERT INTO collections (
      title, slug, description, creator_id,
      collaborative, is_public, is_featured,
      created_at, updated_at
    )
    VALUES (
      'E2E Worker Collection',
      'e2e-worker-collection-' || worker_user.id,
      'Reserved collection for add-to-collection E2E smoke tests (PSY-455).',
      worker_user.id,
      false, true, false,
      NOW(), NOW()
    );
  END LOOP;
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
  worker_user RECORD;
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

  -- 3) (PSY-503) Legacy pending_venue_edits seed removed. Venue edits now
  -- flow through pending_entity_edits; the deleted admin/venue-edits.spec.ts
  -- was the only consumer of this seed.

  -- 4) Approved show submitted by each worker-user (for "my submissions" test).
  -- PSY-431: one submission per worker-user so my-submissions.spec.ts works
  -- regardless of which worker the test lands on.
  FOR worker_user IN
    SELECT id, email FROM users WHERE email LIKE 'e2e-user%@test.local'
  LOOP
    INSERT INTO shows (title, event_date, city, state, status, source, submitted_by, created_at, updated_at, slug)
    VALUES (
      'E2E My Submitted Show (' || worker_user.email || ')',
      NOW() + INTERVAL '85 days',
      'Phoenix', 'AZ', 'approved', 'user', worker_user.id,
      NOW(), NOW(),
      'e2e-my-submitted-show-' || worker_user.id
    )
    RETURNING id INTO s_id;
    INSERT INTO show_venues (show_id, venue_id) VALUES (s_id, v_id);
    -- PSY-636 / PSY-576 carveout — DO NOT naively populate this row's denorm
    -- cols. PSY-576 added denormalized event_date + venue_id to show_artists
    -- plus a partial unique index on (artist_id, venue_id, event_date)
    -- `WHERE event_date IS NOT NULL AND venue_id IS NOT NULL`. This seed leaves
    -- those denorm cols NULL on every show_artists row, so the partial index
    -- excludes them and they pass straight through. That is INTENTIONAL here:
    -- every iteration of this loop inserts one distinct show per worker-user
    -- but reuses the SAME (artist_id=a_id, venue_id=v_id, event_date=NOW()+85d)
    -- triple, so populating the denorm cols would make all of these rows
    -- collide on the unique index and fail the seed. If a future change needs
    -- the denorm cols populated, first give each iteration a DISTINCT
    -- event_date (e.g. stagger by minute) on both the shows row above and this
    -- show_artists row. See PSY-636 (and PSY-628 for the partial-index design).
    INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (s_id, a_id, 0, 'headliner');
  END LOOP;

END $$;
SQL

echo "==> Seeding parent comments for comments E2E spec (PSY-456)..."
# Pre-seed a parent comment on the vote and reply reserved venues so
# the vote and reply tests don't have to create their own parent first
# (avoids doubling mutations + hitting the 60s per-entity cooldown).
# Authored by the admin user so they have no tier/rate-limit gating
# regardless of which worker runs the test.
psql -v ON_ERROR_STOP=1 "$E2E_DB_URL" <<'SQL'
DO $$
DECLARE
  admin_user_id INTEGER;
  vote_venue_id INTEGER;
  reply_venue_id INTEGER;
BEGIN
  SELECT id INTO admin_user_id FROM users WHERE email = 'e2e-admin@test.local';
  SELECT id INTO vote_venue_id FROM venues WHERE slug = 'e2e-comment-vote';
  SELECT id INTO reply_venue_id FROM venues WHERE slug = 'e2e-comment-reply';

  -- Vote target: a single admin-authored comment that every worker can
  -- upvote. Vote rows are keyed by (comment_id, user_id), so parallel
  -- workers each add their own vote without colliding.
  INSERT INTO comments (
    entity_type, entity_id, kind, user_id,
    body, body_html, visibility, reply_permission,
    created_at, updated_at
  )
  VALUES (
    'venue', vote_venue_id, 'comment', admin_user_id,
    'E2E vote-target seed comment', '<p>E2E vote-target seed comment</p>',
    'visible', 'anyone',
    NOW(), NOW()
  );

  -- Reply parent: depth-0 admin-authored comment so workers can reply.
  -- Each worker replies with its own per-worker user, so the per-entity
  -- cooldown is per-user and doesn't collide.
  INSERT INTO comments (
    entity_type, entity_id, kind, user_id,
    body, body_html, visibility, reply_permission,
    created_at, updated_at
  )
  VALUES (
    'venue', reply_venue_id, 'comment', admin_user_id,
    'E2E reply-parent seed comment', '<p>E2E reply-parent seed comment</p>',
    'visible', 'anyone',
    NOW(), NOW()
  );
END $$;
SQL

echo "==> E2E database setup complete!"
