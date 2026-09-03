import { describe, it, expect } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

/**
 * The drift guard for the auth gate.
 *
 * Nine components each hand-rolled the same three-state branch (bail while
 * pending, push to /auth when settled anonymous, act when authenticated) and
 * the returnTo formula had already split into two answers, four of them
 * dropping the query string. Extracting the branch fixes today's copies; this
 * test is what stops the tenth from being written.
 *
 * Two spellings are refused outside the owners below:
 *   - a hand-built `/auth?returnTo=` href, which is `buildAuthHref`'s job and
 *     is where the query string went missing
 *   - a literal navigation to '/auth', which is a redirect written without the
 *     destination the reader came from
 *
 * The pending half of the rule cannot be caught by grep (a component may read
 * `authStatus === 'pending'` for perfectly good rendering reasons), so it is
 * pinned by `useAuthGatedAction.test.tsx` and `useAuthRouteGuard.test.tsx`
 * instead. What this file guarantees is that there is only one place for it
 * to live.
 */

const ROOT = join(import.meta.dirname, '..', '..', '..')
const SCANNED_DIRS = ['app', 'components', 'features', 'lib']

// The modules that OWN a sign-in destination. Everything else asks them.
const OWNERS = new Set(
  [
    'lib/auth-href.ts',
    'lib/hooks/common/useAuthGatedAction.ts',
    'lib/hooks/common/useAuthRouteGuard.ts',
    // The inverse of the contract: it reads returnTo back out and decides
    // whether to trust it, so it names the same route on purpose.
    'app/auth/auth-redirect-utils.ts',
  ].map(p => p.split('/').join(sep))
)

const SKIPPED_DIR_NAMES = new Set(['node_modules', '.next', 'e2e', 'e2e-hydration'])

// `app/auth/**` is the sign-in surface itself. A returnTo naming one of those
// routes is discarded by `sanitizeReturnTo`, so their own recovery links go to
// a bare /auth on purpose.
const AUTH_SURFACE_PREFIX = join('app', 'auth') + sep

// Prose is not code. Both halves of this contract are described in doc
// comments that quote the very spellings under test, and a scan that counted
// those would be a scan nobody could keep green. The `:` guard keeps a `//`
// inside a URL literal from swallowing the rest of its line.
function stripComments(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/.*$/gm, '$1')
}

function sourceFiles(dir: string): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir)) {
    if (SKIPPED_DIR_NAMES.has(entry)) continue
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      out.push(...sourceFiles(full))
      continue
    }
    if (!/\.tsx?$/.test(entry)) continue
    // Test files describe the behaviour rather than implementing it, so they
    // are allowed to name the destinations they assert on.
    if (/\.test\.tsx?$/.test(entry)) continue
    out.push(full)
  }
  return out
}

function offenders(pattern: RegExp): string[] {
  const found: string[] = []
  for (const dir of SCANNED_DIRS) {
    for (const file of sourceFiles(join(ROOT, dir))) {
      const rel = relative(ROOT, file)
      if (OWNERS.has(rel)) continue
      if (pattern.test(stripComments(readFileSync(file, 'utf8')))) {
        found.push(rel)
      }
    }
  }
  return found.sort()
}

describe('auth gate ownership', () => {
  it('has exactly one builder for the /auth?returnTo= href', () => {
    // Compose the href through `buildAuthHref`, and get the destination from
    // `useAuthGatedAction` (a control) or `useAuthRouteGuard` (a page).
    expect(offenders(/\/auth\?returnTo=/)).toEqual([])
  })

  it('has no component navigating to a bare /auth of its own', () => {
    // `router.push('/auth')` / `redirect('/auth')` and friends: a sign-in
    // navigation with no way back to the page the reader was on.
    const bare = offenders(
      /(?:push|replace|redirect)\(\s*['"`]\/auth['"`]/
    ).filter(rel => !rel.startsWith(AUTH_SURFACE_PREFIX))
    expect(bare).toEqual([])
  })
})
