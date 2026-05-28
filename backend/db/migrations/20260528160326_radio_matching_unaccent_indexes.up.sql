-- PSY-886: Enable diacritic-insensitive name lookups for radio matching.
--
-- The unaccent extension folds Unicode marks (e.g. "José" -> "Jose"); paired
-- with an expression index this lets the radio matching engine match plays
-- against artists / aliases / releases / labels using diacritic-insensitive
-- equality without sequential scans. See backend/internal/services/catalog/
-- radio_matching.go for the corresponding WHERE clauses.

CREATE EXTENSION IF NOT EXISTS unaccent;

-- Postgres ships unaccent() as STABLE (not IMMUTABLE) because its dictionary
-- is a file that can be reloaded. Expression indexes require IMMUTABLE, so
-- wrap it in a SQL function marked IMMUTABLE. We do this once at the schema
-- level so all four indexes can reuse the same expression. Callers (Go-side
-- WHERE clauses in radio_matching.go) must also use immutable_unaccent so
-- the planner can match the index expression.
--
-- We accept the documented trade-off: if the unaccent dictionary file is
-- ever re-tuned in-place, existing indexed values will not be rewritten
-- until the indexes are REINDEXed. The default `unaccent.rules` file is
-- effectively static, so this risk is theoretical.
CREATE OR REPLACE FUNCTION immutable_unaccent(text)
  RETURNS text
  AS $$ SELECT public.unaccent($1) $$
  LANGUAGE SQL IMMUTABLE PARALLEL SAFE STRICT;

-- Expression indexes so `immutable_unaccent(LOWER(col)) = immutable_unaccent(LOWER(?))`
-- is indexable. Plain CREATE INDEX (not CONCURRENTLY) because:
--   * The PSY-886 AC explicitly permits non-CONCURRENTLY for small tables,
--     and at the time of writing all four tables are tiny (< 500 rows).
--   * golang-migrate v4 wraps each .up.sql file in a single transaction
--     when the file contains multiple statements. CREATE INDEX CONCURRENTLY
--     is incompatible with transactions, so a multi-statement file forces
--     plain CREATE INDEX or per-statement files. We chose the former
--     because the table sizes don't warrant the operational cost.
--   * Existing single-statement migrations (000027, 20260502023011) use
--     CONCURRENTLY because they are alone in their file. The PSY-413
--     CONCURRENTLY CI guard targets that single-statement convention.
-- If any of these tables grow large enough to need an online build (~1M+
-- rows), split each CREATE INDEX into its own follow-up migration with
-- CONCURRENTLY restored.
CREATE INDEX IF NOT EXISTS idx_artists_name_unaccent_lower
  ON artists (immutable_unaccent(LOWER(name)));

CREATE INDEX IF NOT EXISTS idx_artist_aliases_alias_unaccent_lower
  ON artist_aliases (immutable_unaccent(LOWER(alias)));

CREATE INDEX IF NOT EXISTS idx_releases_title_unaccent_lower
  ON releases (immutable_unaccent(LOWER(title)));

CREATE INDEX IF NOT EXISTS idx_labels_name_unaccent_lower
  ON labels (immutable_unaccent(LOWER(name)));
