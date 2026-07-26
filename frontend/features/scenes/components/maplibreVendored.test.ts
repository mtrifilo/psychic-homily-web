import { describe, it, expect } from 'vitest'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import path from 'node:path'

/**
 * Guard for the vendored MapLibre worker assets (PSY-1537 spike finding).
 *
 * maplibre-gl v6's default worker resolution is broken under Turbopack:
 * `import.meta.url` gets rewritten to a `file://…node_modules…` path, and the
 * emitted worker media copy imports an unhashed `./maplibre-gl-shared.mjs`
 * that can't resolve against its hashed sibling. The failure mode is a SILENT
 * PERMANENT HANG — the raster basemap renders, but GeoJSON sources never
 * parse, no dots appear, `idle` never fires, and zero errors log.
 *
 * The fix is byte-identical copies of the dist worker + shared modules served
 * from `public/maplibre/` (same-origin, unhashed names so the worker's
 * relative `./maplibre-gl-shared.mjs` import resolves), pointed at via
 * `setWorkerUrl` before any Map is constructed (GlobeCanvas module scope).
 *
 * This test pins the vendored copies to the INSTALLED package: a maplibre-gl
 * upgrade that forgets to re-vendor fails here instead of shipping a silently
 * dead map. On failure, re-copy:
 *
 *   cp node_modules/maplibre-gl/dist/maplibre-gl-worker.mjs \
 *      node_modules/maplibre-gl/dist/maplibre-gl-shared.mjs public/maplibre/
 */

const FRONTEND_ROOT = path.resolve(__dirname, '../../..')

const VENDORED_FILES = ['maplibre-gl-worker.mjs', 'maplibre-gl-shared.mjs']

function sha256(filePath: string): string {
  return createHash('sha256').update(readFileSync(filePath)).digest('hex')
}

describe('vendored maplibre worker assets', () => {
  it.each(VENDORED_FILES)(
    'public/maplibre/%s is byte-identical to the installed maplibre-gl dist copy',
    (file) => {
      const vendored = path.join(FRONTEND_ROOT, 'public/maplibre', file)
      const dist = path.join(
        FRONTEND_ROOT,
        'node_modules/maplibre-gl/dist',
        file,
      )
      expect(sha256(vendored), `re-vendor ${file} (see file doc)`).toBe(
        sha256(dist),
      )
    },
  )
})
