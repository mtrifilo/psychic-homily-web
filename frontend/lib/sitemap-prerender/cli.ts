/**
 * Post-`next build` gate: refuse to ship a sitemap with no stale-serving
 * fallback.
 *
 *   bun run lib/sitemap-prerender/cli.ts
 *
 * Chained after `next build` in the `build` npm script, which is also Vercel's
 * `buildCommand` (frontend/vercel.json) — so a failure here fails the deploy
 * and leaves the previous deployment serving its own prerendered sitemap.
 *
 * Lives under lib/ rather than scripts/ for the reason lib/sitemap-monitor/cli.ts
 * already records: tsconfig.json excludes `scripts`, so a checker written there
 * would itself be exempt from `bun run typecheck` and `bun run lint`.
 *
 * There is deliberately no env-var opt-out. The only place this runs is Vercel's
 * buildCommand, so the only place such a var could be set persistently is the
 * Vercel project environment — one dashboard toggle on a bad night would
 * silently restore the exact failure mode this exists to stop, leaving no trace
 * in the repo. Building locally without a backend is still easy and cannot leak
 * to a deploy; `formatShardFailures` prints how.
 *
 * The policy, the measurements behind it, and why existence is the whole
 * assertion live in ./check.ts. This file is only the I/O.
 */
import { readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { ALL_SHARD_IDS } from '@/app/sitemap-shards'
import { API_BASE_URL } from '@/lib/api-base'
import {
  findShardsWithoutFallback,
  formatPendingShards,
  formatShardFailures,
  partitionShardFailures,
  type FamilyVerdict,
  type PrerenderManifestLike,
} from './check'

const BUILD_DIR = join(import.meta.dirname, '..', '..', '.next')
const MANIFEST_PATH = join(BUILD_DIR, 'prerender-manifest.json')

function fail(message: string): never {
  console.error(`\n${message}\n`)
  process.exit(1)
}

function readManifest(): PrerenderManifestLike {
  try {
    return JSON.parse(readFileSync(MANIFEST_PATH, 'utf8')) as PrerenderManifestLike
  } catch (error) {
    // A missing or unreadable manifest is an error, never a skip: silence here
    // would defeat the whole gate on exactly the builds it needs to inspect.
    const hint =
      (error as NodeJS.ErrnoException)?.code === 'ENOENT'
        ? 'Run it after `next build`, from the frontend directory.'
        : String(error)
    fail(`Sitemap prerender check could not read ${MANIFEST_PATH}.\n${hint}`)
  }
}

// An empty file is a rendered-but-empty document, which is not a usable
// fallback, so size is part of "has a body" rather than a separate check.
const hasBody = (bodyPath: string): boolean =>
  (statSync(join(BUILD_DIR, bodyPath), { throwIfNoEntry: false })?.size ?? 0) > 0

/**
 * How long the gate will wait for the backend to explain a missing shard.
 *
 * Short on purpose. This runs only when a shard has ALREADY failed, and the
 * question is a cheap one the backend answers from its request validator; a
 * backend too slow to answer it is a backend that cannot excuse anything, which
 * is the same verdict a timeout produces here.
 */
const PROBE_TIMEOUT_MS = 5_000

/**
 * Ask the backend whether it serves a family at all.
 *
 * The build has already run, so this cannot use the route's fetch — it goes
 * straight to the same endpoint with the same base URL the build used. Anything
 * other than a definite "I do not serve that" is `unreachable`, which excuses
 * nothing: a probe that errors, times out, 404s or 5xxs leaves the shard
 * blocking, so a genuine outage still fails the build.
 */
async function probeFamily(shardId: string): Promise<FamilyVerdict> {
  const url = `${API_BASE_URL}/sitemap/entries?family=${encodeURIComponent(shardId)}`
  try {
    const res = await fetch(url, {
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    })
    // Read the status BEFORE releasing the body. Only the status decides the
    // verdict, and a rejected `cancel()` inside this try would otherwise turn a
    // perfectly good answer into `unreachable`, which blocks the deploy under a
    // message blaming the backend.
    const { status, ok } = res
    // Released rather than left dangling: nothing reads it, and an unconsumed
    // body holds its stream open for the rest of the process.
    await res.body?.cancel()
    // Keep this list identical to UNKNOWN_FAMILY_STATUSES in app/sitemap.ts:
    // the two answer the same question, one at render time and one after the
    // build, and a shard excused here must be one the route degrades to empty.
    if (status === 400 || status === 422) return 'unknown'
    return ok ? 'served' : 'unreachable'
  } catch {
    return 'unreachable'
  }
}

const manifest = readManifest()
const failures = findShardsWithoutFallback(manifest, hasBody)

const { blocking, pending } = await partitionShardFailures(failures, probeFamily)

if (blocking.length > 0) {
  fail(formatShardFailures(blocking, manifest))
}

if (pending.length > 0) {
  console.warn(`\n${formatPendingShards(pending)}\n`)
}

console.log(
  `Sitemap prerender check OK: ${ALL_SHARD_IDS.length - pending.length} of ` +
    `${ALL_SHARD_IDS.length} shards have a fallback document` +
    (pending.length > 0
      ? `; ${pending.length} awaiting a backend deploy (see above).`
      : '.')
)
