import { describe, expect, it } from 'vitest'

import robots from './robots'

const NON_PUBLIC_PATHS = [
  '/admin/',
  '/profile/',
  '/auth/',
  '/submissions/',
  '/shows/submit/',
  '/library/',
  '/verify-email/',
]

function rulesOf() {
  const { rules } = robots()
  if (!Array.isArray(rules)) throw new Error('expected an array of rules')
  return rules
}

function agentsOf(rule: { userAgent?: string | string[] }) {
  const ua = rule.userAgent ?? []
  return Array.isArray(ua) ? ua : [ua]
}

describe('robots.txt rules', () => {
  it('keeps the wildcard group allowing everything except the non-public paths', () => {
    const wildcard = rulesOf().find(rule => agentsOf(rule).includes('*'))

    expect(wildcard).toBeDefined()
    expect(wildcard?.allow).toBe('/')
    expect(wildcard?.disallow).toEqual(NON_PUBLIC_PATHS)
  })

  // A crawler obeys only the most specific group naming it and ignores
  // `User-Agent: *` entirely, so any group that grants access must restate the
  // disallow list. A comment cannot fail CI; this can.
  it('restates the non-public disallow list in every group that allows crawling', () => {
    for (const rule of rulesOf()) {
      if (!rule.allow) continue
      expect(rule.disallow, `agents ${agentsOf(rule).join(', ')}`).toEqual(NON_PUBLIC_PATHS)
    }
  })

  it('disallows the whole site for training crawlers', () => {
    const training = rulesOf().filter(rule => !rule.allow)

    expect(training).not.toHaveLength(0)
    for (const rule of training) {
      expect(rule.disallow).toBe('/')
    }
  })

  it('never lists the same agent as both allowed and disallowed', () => {
    const allowed = rulesOf().filter(r => r.allow).flatMap(agentsOf)
    const disallowed = rulesOf().filter(r => !r.allow).flatMap(agentsOf)

    expect(allowed.filter(agent => disallowed.includes(agent))).toEqual([])
  })

  it('blocks training crawlers without touching the search crawlers they shadow', () => {
    const disallowed = rulesOf()
      .filter(r => !r.allow)
      .flatMap(agentsOf)

    // Google-Extended and Applebot-Extended are training-only controls. Naming
    // Googlebot or Applebot here instead would deindex the site.
    expect(disallowed).toContain('Google-Extended')
    expect(disallowed).toContain('Applebot-Extended')
    expect(disallowed).not.toContain('Googlebot')
    expect(disallowed).not.toContain('Applebot')
  })

  it('points at the sitemap', () => {
    expect(robots().sitemap).toBe('https://psychichomily.com/sitemap.xml')
  })
})
