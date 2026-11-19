import { readdirSync } from 'node:fs'
import { resolve } from 'node:path'

import { withSentryConfig } from '@sentry/nextjs'
import { PHASE_DEVELOPMENT_SERVER } from 'next/constants'
import { resolveRouteData } from 'next/dist/build/webpack/loaders/metadata/resolve-route-data'
import { describe, expect, it, vi } from 'vitest'

import nextConfigForPhase from '../next.config'
import robots from './robots'

// test/setup.ts stubs withSentryConfig; next.config calls it once at module
// scope. Read here, before any afterEach clears the recorded calls.
const sentryBuildOptions = vi.mocked(withSentryConfig).mock.calls[0]?.[1]

const SENTRY_OPTIONS_MISSING =
  'next.config.ts must call withSentryConfig at module scope with a string tunnelRoute; ' +
  'this test reads the options from the test/setup.ts stub'

// Deliberate copies of the source values, not imports. A change to the policy
// in robots.ts must be re-approved here rather than silently followed — that is
// the whole point of these assertions. Do not "fix" a failure by importing the
// constants or by comparing one rule against another.
const NON_PUBLIC_PATHS = [
  '/admin/',
  '/profile/',
  '/auth/',
  '/submissions/',
  '/shows/submit/',
  '/library/',
  '/verify-email/',
]

const RSC_PAYLOAD_PATTERNS = ['/*?_rsc=', '/*&_rsc=']

const RENDER_HELPER_ENDPOINTS = ['/monitoring', '/_vercel/insights/', '/api/bandcamp/']

const PUBLIC_DISALLOW = [...NON_PUBLIC_PATHS, ...RSC_PAYLOAD_PATTERNS, ...RENDER_HELPER_ENDPOINTS]

// Every one of these answers with a citation and a link back. Moving any of
// them into the disallow list is the most likely wrong edit to this file: they
// carry AI-vendor names and read like training crawlers to anyone who has not
// checked the operator docs.
const MUST_STAY_ALLOWED = [
  'OAI-SearchBot',
  'ChatGPT-User',
  'Claude-SearchBot',
  'Claude-User',
  'PerplexityBot',
  'Perplexity-User',
  'Meta-WebIndexer',
  'Meta-ExternalFetcher',
  // Dual-purpose tokens, allowed on the retrieval-wins rule. Google-Extended
  // also gates Gemini grounding, and Meta-ExternalAgent also gates indexing.
  'Meta-ExternalAgent',
  'Google-Extended',
  // Plain search crawlers. Blocking these would deindex the site.
  'Applebot',
  'Googlebot',
  'Bingbot',
]

const MUST_STAY_DISALLOWED = ['GPTBot', 'ClaudeBot', 'Applebot-Extended']

function rulesOf() {
  const { rules } = robots()
  if (!Array.isArray(rules)) throw new Error('expected an array of rules')
  return rules
}

// Mirrors Next's serializer, which treats a missing userAgent as '*'
// (`resolveArray(rule.userAgent || ['*'])`). Defaulting to [] instead would let
// an untagged rule silently escape the wildcard assertions below.
function agentsOf(rule: { userAgent?: string | string[] }) {
  const ua = rule.userAgent ?? ['*']
  return Array.isArray(ua) ? ua : [ua]
}

function allowedAgents() {
  return rulesOf()
    .filter(rule => rule.allow)
    .flatMap(agentsOf)
}

function disallowedAgents() {
  return rulesOf()
    .filter(rule => !rule.allow)
    .flatMap(agentsOf)
}

/** Named groups win outright, so an agent named nowhere is governed by '*'. */
function isGovernedByWildcard(agent: string) {
  return !rulesOf().some(rule =>
    agentsOf(rule).some(named => named.toLowerCase() === agent.toLowerCase())
  )
}

describe('robots.txt rules', () => {
  it('keeps exactly one wildcard group, allowing everything but the non-public paths, RSC payloads and render helpers', () => {
    const wildcards = rulesOf().filter(rule => agentsOf(rule).includes('*'))

    // Two '*' groups would silently merge under RFC 9309 and change the
    // effective policy, so the count matters as much as the contents.
    expect(wildcards).toHaveLength(1)
    expect(wildcards[0].allow).toBe('/')
    expect(wildcards[0].disallow).toEqual(PUBLIC_DISALLOW)
  })

  // A crawler obeys only the most specific group naming it and ignores
  // `User-Agent: *` entirely, so a named group inherits nothing. Every group
  // that grants access must therefore carry the identical rule body — for all
  // fields, not just disallow. A comment cannot fail CI; this can.
  it('gives every allowing group a byte-identical rule body', () => {
    const bodies = rulesOf()
      .filter(rule => rule.allow)
      .map(rule => ({ allow: rule.allow, disallow: rule.disallow }))

    expect(bodies.length).toBeGreaterThan(1)
    for (const body of bodies) {
      expect(body).toEqual(bodies[0])
    }
    expect(bodies[0]).toEqual({ allow: '/', disallow: PUBLIC_DISALLOW })
  })

  // robots.ts holds the tunnel path as its own constant. This pins it to the
  // `tunnelRoute` next.config.ts gives Sentry, so a change to either fails here.
  it('disallows the Sentry tunnel route next.config.ts sets, in every allowing group', () => {
    const tunnelRoute = sentryBuildOptions?.tunnelRoute

    expect(typeof tunnelRoute, SENTRY_OPTIONS_MISSING).toBe('string')
    const allowing = rulesOf().filter(rule => rule.allow)
    expect(allowing.length).toBeGreaterThan(0)
    for (const rule of allowing) {
      expect(rule.disallow).toContain(tunnelRoute)
    }
  })

  it('disallows the whole site for training crawlers', () => {
    const training = rulesOf().filter(rule => !rule.allow)

    expect(training).not.toHaveLength(0)
    for (const rule of training) {
      expect(rule.disallow).toBe('/')
    }
  })

  // The failure this guards against is a MOVE between the two lists, which
  // leaves the intersection empty and so slips past a both-lists check.
  it('keeps every citing agent out of the disallow list', () => {
    const disallowed = disallowedAgents().map(a => a.toLowerCase())

    for (const agent of MUST_STAY_ALLOWED) {
      expect(disallowed, `${agent} must never be disallowed`).not.toContain(agent.toLowerCase())
    }
  })

  it('allows every citing agent, explicitly or via the wildcard', () => {
    const allowed = allowedAgents().map(a => a.toLowerCase())

    for (const agent of MUST_STAY_ALLOWED) {
      const reached = allowed.includes(agent.toLowerCase()) || isGovernedByWildcard(agent)
      expect(reached, `${agent} must remain reachable`).toBe(true)
    }
  })

  it('keeps the training crawlers blocked', () => {
    const disallowed = disallowedAgents().map(a => a.toLowerCase())

    for (const agent of MUST_STAY_DISALLOWED) {
      expect(disallowed, `${agent} must stay blocked`).toContain(agent.toLowerCase())
    }
  })

  // Token matching is case-insensitive per RFC 9309, and vendor docs often
  // render tokens lowercase, so compare case-insensitively.
  it('never lists the same agent as both allowed and disallowed', () => {
    const allowed = allowedAgents().map(a => a.toLowerCase())
    const disallowed = disallowedAgents().map(a => a.toLowerCase())

    expect(allowed.filter(agent => disallowed.includes(agent))).toEqual([])
  })

  // Deliberately /sitemap-index, NOT /sitemap.xml. Under generateSitemaps()
  // the child documents live at /sitemap/{id}.xml and Next emits no index for
  // that shape, so app/sitemap-index/route.ts serves the index and robots
  // points there (PSY-1622). robots.ts is authoritative here — if this
  // assertion fails, check whether the sitemap shape changed before editing it.
  it('points at the sitemap index', () => {
    expect(robots().sitemap).toBe('https://psychichomily.com/sitemap-index')
  })
})

// The served /robots.txt is the output of this same Next function applied to
// robots(), so these assertions read the bytes a crawler receives.
function renderedRobotsTxt() {
  return resolveRouteData(robots(), 'robots')
}

const USER_AGENT_PREFIX = 'User-Agent: '

interface RenderedGroup {
  agents: string[]
  rules: string[]
}

function renderedGroups(): RenderedGroup[] {
  return renderedRobotsTxt()
    .split('\n\n')
    .map(block => block.split('\n').filter(Boolean))
    .filter(lines => lines.some(line => line.startsWith(USER_AGENT_PREFIX)))
    .map(lines => {
      const group: RenderedGroup = { agents: [], rules: [] }
      for (const line of lines) {
        if (line.startsWith(USER_AGENT_PREFIX)) {
          group.agents.push(line.slice(USER_AGENT_PREFIX.length))
        } else {
          group.rules.push(line)
        }
      }
      return group
    })
}

function renderedGroupFor(agent: string): RenderedGroup {
  const group = renderedGroups().find(candidate => candidate.agents.includes(agent))
  if (!group) throw new Error(`no rendered group names ${agent}`)
  return group
}

/**
 * A robots.txt path pattern as Google and RFC 9309 read it: anchored at the
 * start of the path-plus-query, `*` matches any run of characters, a trailing
 * `$` anchors the end, and every other character is literal.
 */
function patternMatches(pattern: string, pathAndQuery: string) {
  const anchored = pattern.endsWith('$')
  const body = anchored ? pattern.slice(0, -1) : pattern
  const source = body
    .split('*')
    .map(part => part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
    .join('.*')
  return new RegExp(`^${source}${anchored ? '$' : ''}`).test(pathAndQuery)
}

/**
 * Whether a group lets a crawler fetch a URL: the longest matching pattern
 * decides, and on a tie the Allow wins (the least restrictive rule).
 */
function groupAllows(rules: string[], pathAndQuery: string) {
  let best: { length: number; allow: boolean } | null = null
  for (const line of rules) {
    const match = /^(Allow|Disallow): (.+)$/.exec(line)
    if (!match || !patternMatches(match[2], pathAndQuery)) continue
    const candidate = { length: match[2].length, allow: match[1] === 'Allow' }
    if (
      !best ||
      candidate.length > best.length ||
      (candidate.length === best.length && candidate.allow)
    ) {
      best = candidate
    }
  }
  return best ? best.allow : true
}

/**
 * The names that can appear as a URL's first path segment under a directory:
 * its entries, with App Router route groups `(name)` opened, since a group
 * adds no segment of its own.
 */
function firstUrlSegments(dir: string): string[] {
  return readdirSync(resolve(process.cwd(), dir)).flatMap(entry =>
    entry.startsWith('(') && entry.endsWith(')') ? firstUrlSegments(`${dir}/${entry}`) : [entry]
  )
}

const PUBLIC_RULE_LINES = [
  'Allow: /',
  ...PUBLIC_DISALLOW.map(path => `Disallow: ${path}`),
]

// RSC payload URLs with `_rsc` as the only query parameter, after another
// query parameter, and on the site root.
const RSC_PAYLOAD_URLS = [
  '/venues/troubadour-west-hollywood-ca?_rsc=113fy',
  '/artists/the-stellar-bandits?_rsc=cqffb',
  '/shows?_rsc=1qv81',
  '/auth?_rsc=1qv81',
  '/shows?city=phoenix-az&_rsc=1qv81',
  '/search?q=slift&page=2&_rsc=abc12',
  '/?_rsc=abc12',
]

// URLs a crawler needs or indexes. None carries the RSC parameter, so none may
// be newly disallowed, including near misses that contain the letters `_rsc`.
const MUST_STAY_FETCHABLE = [
  '/',
  '/artists/slift',
  '/artists/slift?tab=releases',
  '/shows',
  '/shows?city=phoenix-az',
  '/shows/slift-at-the-rebel-lounge-2026-11-01/opengraph-image',
  '/shows/slift-at-the-rebel-lounge-2026-11-01/opengraph-image?a1b2c3d4',
  '/_next/static/chunks/app/layout-0123abcd.js',
  '/_next/static/css/app-0123abcd.css',
  '/_next/static/media/font-0123abcd.woff2',
  '/_next/image?url=%2Fimages%2Flogo.png&w=640&q=75',
  '/favicon.ico',
  '/sitemap-index',
  '/search?q=_rsc',
  '/search?q=a_rsc=b',
  '/artists/_rsc=band',
]

// The shapes a renderer requests: the Sentry tunnel with its query string and
// with the trailing slash its rewrite also accepts, the Web Analytics script
// and beacons, and the Bandcamp album-id lookup.
const RENDER_HELPER_URLS = [
  '/monitoring?o=4508000000000000&p=4508000000000001',
  '/monitoring?o=4508000000000000&p=4508000000000001&r=us',
  '/monitoring/?o=4508000000000000&p=4508000000000001',
  '/_vercel/insights/script.js',
  '/_vercel/insights/view',
  '/_vercel/insights/event',
  '/api/bandcamp/album-id?url=https%3A%2F%2Fslift.bandcamp.com%2Falbum%2Fituri',
]

// URLs the render-helper patterns must not over-match: the Speed Insights
// paths share the `/_vercel/` prefix but not `/_vercel/insights/`, and the
// pages merely contain the words. Speed Insights is outside the render-helper
// list by its own scope (robots.ts); this list checks the matching only.
const RENDER_HELPER_NEAR_MISSES = [
  '/_vercel/speed-insights/script.js',
  '/_vercel/speed-insights/vitals',
  '/artists/monitoring',
  '/search?q=/monitoring',
  '/tags/bandcamp',
  '/search?q=/api/bandcamp/',
]

describe('rendered robots.txt', () => {
  it.each(['*', 'OAI-SearchBot'])('renders the full public rule body verbatim for %s', agent => {
    expect(renderedGroupFor(agent).rules).toEqual(PUBLIC_RULE_LINES)
  })

  it('leaves the training-crawler group disallowing the whole site', () => {
    expect(renderedGroupFor('GPTBot').rules).toEqual(['Disallow: /'])
  })

  // Both allowing groups render the same body (pinned above), so matching URLs
  // against the rendered wildcard group covers the retrieval group too.
  it.each([
    ['RSC payload', RSC_PAYLOAD_URLS],
    ['render-helper endpoint', RENDER_HELPER_URLS],
  ])('blocks every %s URL', (_, urls) => {
    const { rules } = renderedGroupFor('*')
    for (const url of urls) {
      expect(groupAllows(rules, url), `${url} must be disallowed`).toBe(false)
    }
  })

  it.each([
    ['pages and assets', MUST_STAY_FETCHABLE],
    ['URLs the render-helper patterns must not over-match', RENDER_HELPER_NEAR_MISSES],
  ])('keeps %s fetchable', (_, urls) => {
    const { rules } = renderedGroupFor('*')
    for (const url of urls) {
      expect(groupAllows(rules, url), `${url} must stay allowed`).toBe(true)
    }
  })

  // The tunnel pattern is a bare prefix, so any URL whose path starts with the
  // tunnel route is disallowed with it. This checks the static top-level app
  // entries and public files, refuses a dynamic root segment (which could
  // resolve to such a path), and checks the literal start of every rewrite and
  // redirect source in next.config.ts. A source that opens with a parameter is
  // not inspected.
  it('leaves no static route, public file or config source under the tunnel prefix', async () => {
    const tunnelRoute = sentryBuildOptions?.tunnelRoute
    expect(typeof tunnelRoute, SENTRY_OPTIONS_MISSING).toBe('string')
    const prefix = String(tunnelRoute)
    const firstSegment = prefix.slice(1)

    const segments = [...firstUrlSegments('app'), ...firstUrlSegments('public')]
    expect(segments.filter(segment => segment.startsWith(firstSegment))).toEqual([])
    expect(
      firstUrlSegments('app').filter(segment => segment.startsWith('[')),
      'a dynamic root segment can serve paths under the bare tunnel prefix'
    ).toEqual([])

    const config = nextConfigForPhase(PHASE_DEVELOPMENT_SERVER)
    const rewrites = (await config.rewrites?.()) ?? []
    const rewriteRules = Array.isArray(rewrites)
      ? rewrites
      : [
          ...(rewrites.beforeFiles ?? []),
          ...(rewrites.afterFiles ?? []),
          ...(rewrites.fallback ?? []),
        ]
    const redirectRules = (await config.redirects?.()) ?? []
    const sources = [...rewriteRules, ...redirectRules].map(rule => rule.source)
    expect(sources.length).toBeGreaterThan(0)
    expect(sources.filter(source => source.startsWith(prefix))).toEqual([])
  })

  // Guards the matcher itself, so a broken helper cannot pass the checks above
  // by matching nothing or everything.
  it('reads robots.txt patterns the way Google documents them', () => {
    expect(patternMatches('/fish', '/fish.php?id=anything')).toBe(true)
    expect(patternMatches('/fish', '/Fish.asp')).toBe(false)
    expect(patternMatches('/*.php$', '/folder/filename.php')).toBe(true)
    expect(patternMatches('/*.php$', '/filename.php?parameters')).toBe(false)
    expect(groupAllows(PUBLIC_RULE_LINES, '/admin/users')).toBe(false)
    expect(groupAllows(PUBLIC_RULE_LINES, '/admin')).toBe(true)
  })

  // The rows of Google's "order of precedence for rules" table.
  it.each([
    [['Allow: /p', 'Disallow: /'], '/page', true],
    [['Allow: /folder', 'Disallow: /folder'], '/folder/page', true],
    [['Allow: /page', 'Disallow: /*.htm'], '/page.htm', false],
    [['Allow: /page', 'Disallow: /*.ph'], '/page.php5', true],
    [['Allow: /$', 'Disallow: /'], '/', true],
    [['Allow: /$', 'Disallow: /'], '/page.htm', false],
  ])('resolves %j for %s the way Google documents', (rules, url, allowed) => {
    expect(groupAllows(rules, url)).toBe(allowed)
  })
})
