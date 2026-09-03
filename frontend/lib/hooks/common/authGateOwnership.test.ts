import { describe, it, expect } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

/**
 * The drift guard for the auth gate.
 *
 * The sign-in destination is one formula in one place. A second copy of it is
 * free to disagree with the first, and the disagreement is invisible on any
 * page that carries no query string, so this refuses the copy rather than
 * waiting for the divergence.
 *
 * Three things are refused outside the owners below:
 *   - a hand-built `/auth?returnTo=` href, which is `buildAuthHref`'s job
 *   - a bare `'/auth'` string, whether pushed, redirected or handed to an
 *     `href`, which is a sign-in route written without the destination the
 *     reader came from. `AUTH_PATH` is the spelling for a site that genuinely
 *     has no destination to carry.
 *   - an import of `buildAuthHref` from anywhere but the small set of files
 *     that legitimately compose an href themselves. That is the rule the other
 *     two cannot express: `buildAuthHref(pathname)` inside a click handler is
 *     the query-string loss this family exists to fix, and it names neither
 *     forbidden string.
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

// The files that compose a sign-in href themselves rather than asking a hook.
// Every one is a RENDER-time link, where `currentLocationReturnTo` cannot read
// a query string, and each is here for one of two reasons: it sends the reader
// back to wherever they are (the pathname, the best a render can do), or it
// names a FIXED destination that is not the current page at all. A click
// handler is neither, and belongs in `useAuthGatedAction`. Adding a name here
// is the deliberate act this list exists to require.
const AUTH_HREF_COMPOSERS = new Set(
  [
    // Back to the current pathname.
    'features/auth/components/SignInPrompt.tsx',
    'components/layout/nav/UserMenu.tsx',
    'components/layout/nav/BottomTabBar.tsx',
    // A fixed destination, resolved at module scope.
    'app/shows/submit/page.tsx',
    'app/verify-email/page.tsx',
  ].map(f => f.split('/').join(sep))
)

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

  it('has no component naming a bare /auth of its own', () => {
    // Covers `router.push('/auth')`, `redirect('/auth')` and `href="/auth"`
    // alike: a sign-in route with no way back to the page the reader was on.
    // `AUTH_PATH` is how a site with genuinely nowhere to return says so.
    const bare = offenders(/['"`]\/auth['"`]/).filter(
      rel => !rel.startsWith(AUTH_SURFACE_PREFIX)
    )
    expect(bare).toEqual([])
  })

  it('keeps href composition to the render-time links that need it', () => {
    const composers = offenders(/\bbuildAuthHref\b/).filter(
      rel => !AUTH_HREF_COMPOSERS.has(rel)
    )
    expect(composers).toEqual([])
  })
})
