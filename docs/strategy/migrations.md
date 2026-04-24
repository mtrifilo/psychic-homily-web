# Database Migrations — Strategy and Operating Policy

> Closes PSY-415. The rules for writing migrations in `backend/db/migrations/`, the acknowledged historical exceptions, and the tooling that enforces them.

## TL;DR

- **One concern per migration.** DDL and DML go in separate files.
- **No data-only migrations.** Seed data lives in `cmd/seed` + `backend/internal/seeddata/` (see [PSY-414](https://linear.app/psychic-homily/issue/PSY-414)).
- **Timestamp versioning for new files.** `YYYYMMDDhhmmss_name.up.sql`. Existing `000001`–`000077` stay sequential (see [PSY-416](https://linear.app/psychic-homily/issue/PSY-416)).
- **`CREATE INDEX CONCURRENTLY` must be the only statement in its migration file** (see [PSY-413](https://linear.app/psychic-homily/issue/PSY-413)).
- **Every `*.up.sql` needs a matching `*.down.sql`.** CI migration-reversibility proves up → down → up round-trips on every PR.

## Writing a new migration

1. `go run ./cmd/scaffold <entity>` if you're creating an entity; it emits correctly-shaped migration stubs. Otherwise hand-create two files next to each other with today's UTC timestamp:
   ```
   backend/db/migrations/20260423143022_add_foo.up.sql
   backend/db/migrations/20260423143022_add_foo.down.sql
   ```
2. Keep the file single-concern: either schema changes (DDL) or data changes (DML), not both.
3. If the change requires a backfill, ship it as two migrations: `...create_foo.up.sql` (DDL), then `...backfill_foo.up.sql` (DML) immediately after.
4. Write a real `.down.sql`. CI proves it works; don't punt on reversibility.

## The four rules

### 1. One concern per migration

A migration file either changes schema (DDL) or changes data (DML). Not both.

**Why:** if the DDL succeeds and the DML fails partway, the schema is left in an inconsistent state that's hard to retry cleanly. Splitting makes the failure mode obvious — either the DDL migration applied or it didn't; the DML migration is a separate, retriable step. It also lets down migrations cleanly invert their single operation.

### 2. No data-only migrations

Seed data is mutable and environment-specific. It doesn't belong in `schema_migrations`.

**Where seed data lives instead:** `backend/internal/seeddata/` (shared Go package) is consumed by `backend/cmd/seed/main.go` for local dev and by a generator script that emits SQL for `frontend/e2e/setup-db.sh`. Prod deploys run `cmd/seed` after `migrate up`. See [PSY-414](https://linear.app/psychic-homily/issue/PSY-414) for the full architecture.

**Why:** migrations 000068 / 000069 / 000070 / 000072 / 000073 / 000076 / 000077 are all data-only migrations that accreted because seed data was baked into the schema migrations from day one. PSY-385 (April 2026) is the incident this caused — the seed CLI and the seed migration drifted, and it took four follow-up migrations to repair. Single source of truth in Go prevents this class of bug.

### 3. Timestamp versioning for new migrations

New migration files use UTC timestamp versions: `YYYYMMDDhhmmss_name.up.sql`. Existing files `000001`–`000077` stay as-is (renaming would break `schema_migrations` on deployed environments).

**Why:** sequential integers race when two developers merge migrations the same week. Timestamps cannot collide at second-granularity. golang-migrate sorts versions numerically and timestamps are strictly larger than the highest sequential (`20260423143022` ≫ `77`), so the two formats coexist cleanly in `schema_migrations` (bigint) and in all our tooling.

**What's been verified to handle both formats:**
- CI `migration-lint` — extracts `([0-9]+)_` from filenames ([`.github/workflows/ci.yml:17-31`](../../.github/workflows/ci.yml))
- CI `migration-reversibility` — `sort -n` puts timestamps last ([`.github/workflows/ci.yml:86-97`](../../.github/workflows/ci.yml))
- `internal/testutil/migrations.go` — `sort.Strings` (alphabetical); `'0' < '2'` keeps timestamps after sequential
- `schema_migrations.version` — bigint, easily holds 14-digit timestamps

### 4. `CREATE INDEX CONCURRENTLY` is special

Postgres refuses `CREATE INDEX CONCURRENTLY` inside a transaction. The golang-migrate postgres driver wraps multi-statement migrations in an implicit transaction. So:

- `CREATE INDEX CONCURRENTLY` must be the ONLY statement in its migration file. No comments with side effects, no additional DDL.
- Unit/integration tests don't hit the real migrate CLI — they go through [`internal/testutil/migrations.go:37`](../../backend/internal/testutil/migrations.go) which calls `strings.ReplaceAll(content, "CONCURRENTLY ", "")`. Tests run the index creation non-concurrently; prod runs it concurrently.
- Don't remove the strip workaround without a plan; it's what lets tests use testcontainers safely.

See [`000027_add_index_duplicate_of_show_id.up.sql`](../../backend/db/migrations/000027_add_index_duplicate_of_show_id.up.sql) for the canonical shape.

## Historical exceptions (grandfathered)

Four migrations currently in the repo break the one-concern rule. All four are in prod; splitting them retroactively would leave `schema_migrations` inconsistent across environments, so they're acknowledged and left alone:

| File | DDL | DML | Why it was written this way |
|---|---|---|---|
| `000004_update_venue_constraints.up.sql` | ALTER venues city/state NOT NULL, add `verified`, DROP old constraint, CREATE composite index | UPDATE venues backfill city/state, UPDATE verified=true | NOT NULL constraints require non-null backfill first; verified=true was a retroactive "trust all existing rows" decision |
| `000037_create_user_bookmarks.up.sql` | CREATE user_bookmarks + 5 indexes, DROP old `user_saved_shows` and `user_favorite_venues` tables | INSERT bookmarks from each old table | Merged two tables into a polymorphic one; needed a single atomic cutover |
| `000044_auto_approve_default_false.up.sql` | ALTER venue_source_configs auto_approve default | UPDATE existing rows to false | Policy change that required both new-row default and existing-row correction |
| `000048_add_data_provenance.up.sql` | ALTER + CHECK + INDEX on 6 tables (24 statements) | UPDATE shows.data_source from shows.source | Added provenance columns across all 6 core entities; shows was the only one with a pre-existing `source` to migrate from |

## Tooling reference

| Tool | What it does |
|---|---|
| [`backend/cmd/scaffold/main.go`](../../backend/cmd/scaffold/main.go) | Generates migration + model + contracts + service + handler + frontend feature module for a new entity. Produces timestamp-formatted migration names. |
| [`backend/internal/testutil/migrations.go`](../../backend/internal/testutil/migrations.go) | `RunAllMigrations(t, db, dir)` — used by integration tests via testcontainers. Globs `*.up.sql`, sorts alphabetically, strips `CONCURRENTLY`, executes. |
| [`backend/docker-compose.yml`](../../backend/docker-compose.yml) | Local `migrate` service uses official `migrate/migrate:latest` image. Same flow in `docker-compose.prod.yml` / `.stage.yml` / `.e2e.yml`. |
| [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) | `migration-lint` (duplicate versions, missing down) + `migration-reversibility` (real `migrate` CLI up → down → up against Postgres 18). |

## When in doubt

- Before writing a migration, skim the four rules above.
- Before writing a migration that touches production data, ask: does this go in `cmd/seed` instead?
- Before adding `CREATE INDEX CONCURRENTLY`, confirm it's the only statement in the file.
- If CI migration-reversibility fails, check whether your `.down.sql` actually reverses your `.up.sql` — our tests catch this.
