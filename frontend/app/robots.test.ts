import { describe, expect, it } from 'vitest'

import robots from './robots'

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
  it('keeps exactly one wildcard group, allowing everything but the non-public paths', () => {
    const wildcards = rulesOf().filter(rule => agentsOf(rule).includes('*'))

    // Two '*' groups would silently merge under RFC 9309 and change the
    // effective policy, so the count matters as much as the contents.
    expect(wildcards).toHaveLength(1)
    expect(wildcards[0].allow).toBe('/')
    expect(wildcards[0].disallow).toEqual(NON_PUBLIC_PATHS)
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
    expect(bodies[0]).toEqual({ allow: '/', disallow: NON_PUBLIC_PATHS })
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
