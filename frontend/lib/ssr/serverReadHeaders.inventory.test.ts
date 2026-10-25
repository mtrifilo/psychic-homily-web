import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * Every server-side module that reads the backend sends the server-read secret
 * on each of its fetches, or is named below with the reason it must not.
 *
 * A module counts as a server-side backend reader when it is not a client
 * module (`'use client'`), calls `fetch` (or an injected `fetchImpl`), and
 * addresses the backend (`API_BASE_URL`, `BACKEND_URL`, an endpoint table, a
 * spec's `buildUrl`, or a URL handed to a `fetchImpl` helper). It must call
 * `serverReadHeaders()` at least as often as it calls fetch. That is a textual
 * check: it catches a forgotten module or a new fetch in a known one, not a
 * header built and then dropped, which the per-caller tests cover.
 */
const EXEMPT: Record<string, string> = {
  'app/api/[...path]/route.ts':
    'the dev /api relay forwards a VISITOR request; adding the secret would hand the exemption to the visitor',
  'lib/proxy-revalidation.ts':
    'runs only inside the dev /api relay, after a visitor mutation it is relaying',
  'lib/admin-artist-route.ts': 'PATCH, which the exemption never covers',
  'lib/ai-extraction-throttle.ts': 'POST, which the exemption never covers',
  'lib/api.ts': 'the browser request helper (credentials: include); server reads do not use it',
}

const ROOT = join(__dirname, '..', '..')
const SCANNED = ['app', 'lib', 'features', 'components', 'proxy.ts']

function sourceFiles(path: string): string[] {
  const stats = statSync(path)
  if (stats.isFile()) return /\.(ts|tsx)$/.test(path) ? [path] : []
  return readdirSync(path).flatMap(entry =>
    entry === 'node_modules' || entry === '__tests__' ? [] : sourceFiles(join(path, entry))
  )
}

function isServerBackendReader(source: string): boolean {
  const head = source.split('\n').slice(0, 5).join('\n')
  if (/['"]use client['"]/.test(head)) return false
  if (!/\bfetch(Impl)?\(/.test(source)) return false
  return /API_BASE_URL|BACKEND_URL|Endpoints\b|buildUrl|fetchImpl/.test(source)
}

function count(source: string, pattern: RegExp): number {
  return source.match(pattern)?.length ?? 0
}

const readers = SCANNED.flatMap(entry => sourceFiles(join(ROOT, entry)))
  .filter(file => !/\.test\.tsx?$/.test(file))
  .map(file => ({ file: relative(ROOT, file), source: readFileSync(file, 'utf8') }))
  .filter(({ source }) => isServerBackendReader(source))

describe('server-read secret inventory', () => {
  it('finds the server-side backend readers it guards', () => {
    expect(readers.map(r => r.file)).toEqual(
      expect.arrayContaining([
        'proxy.ts',
        'app/artists/[slug]/page.tsx',
        'app/sitemap.ts',
        'lib/ssr/fetchListPayload.ts',
        'lib/seo/fetchSeoList.ts',
        'features/scenes/scenePeriodApi.ts',
      ])
    )
  })

  it.each(readers.map(r => [r.file, r.source] as const))(
    '%s sends the secret or is exempt for a stated reason',
    (file, source) => {
      if (file in EXEMPT) {
        expect(source).not.toContain('serverReadHeaders(')
        return
      }
      const fetches = count(source, /\bfetch(Impl)?\(/g)
      expect(count(source, /serverReadHeaders\(/g)).toBeGreaterThanOrEqual(fetches)
    }
  )

  it('names no exemption that no longer exists', () => {
    const found = new Set(readers.map(r => r.file))
    expect(Object.keys(EXEMPT).filter(file => !found.has(file))).toEqual([])
  })
})
