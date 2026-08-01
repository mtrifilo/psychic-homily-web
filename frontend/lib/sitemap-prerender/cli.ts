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
import {
  ALL_SHARD_IDS,
  findShardsWithoutFallback,
  formatShardFailures,
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

const manifest = readManifest()
const failures = findShardsWithoutFallback(manifest, hasBody)

if (failures.length > 0) {
  fail(formatShardFailures(failures, manifest))
}

console.log(
  `Sitemap prerender check OK: ${ALL_SHARD_IDS.length} shards have a fallback document.`
)
