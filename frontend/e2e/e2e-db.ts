import { execFileSync } from 'child_process'

/**
 * PSY-1663 — single source of truth for the E2E harness's database.
 *
 * The harness already seeds through `psql` (global-setup -> setup-db.sh), so
 * SQL is an established layer here, not a new one. What is new is that a few
 * specs need to RESTORE a seed row mid-run (see fixtures/seed-restore.ts), and
 * they must aim at exactly the database global setup seeded.
 *
 * The URL is a constant rather than an env lookup on purpose. `DATABASE_URL`
 * commonly points at a developer's dev database in an interactive shell, and
 * Playwright workers inherit that shell's environment — reading it here would
 * make "run E2E in the terminal I was just using" silently rewrite dev rows.
 * The port and credentials are pinned by backend/docker-compose.e2e.yml, which
 * publishes 5433 unconditionally, so there is nothing legitimate to override.
 */
export const E2E_DATABASE_URL =
  'postgres://e2euser:e2epassword@localhost:5433/e2edb?sslmode=disable'

/**
 * runE2ESql executes one statement against the E2E database and returns its
 * output with tuple-only, unaligned formatting (`-tAc`) — i.e. bare values,
 * one row per line, which is what the callers here parse.
 *
 * `ON_ERROR_STOP=1` makes psql exit non-zero on a SQL error; without it psql
 * prints the error and still exits 0, which would turn a broken restore into a
 * silently skipped one.
 *
 * Synchronous by design: callers are test hooks that must not proceed until
 * the database reflects the change, and the whole round trip is a few
 * milliseconds against a local container.
 */
export function runE2ESql(sql: string): string {
  try {
    return execFileSync(
      'psql',
      ['-v', 'ON_ERROR_STOP=1', '-tAc', sql, E2E_DATABASE_URL],
      { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'] },
    ).trim()
  } catch (err) {
    const stderr = (err as { stderr?: Buffer | string }).stderr
    throw new Error(
      `E2E SQL failed against ${E2E_DATABASE_URL}: ${
        stderr ? String(stderr).trim() : (err as Error).message
      }\n  statement: ${sql}\n` +
        `  (psql must be on PATH — the same binary global setup uses to seed; ` +
        `CI installs it via the "Install PostgreSQL client" step.)`,
    )
  }
}
