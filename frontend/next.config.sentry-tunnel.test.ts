import { describe, it, expect, vi } from 'vitest'
import type { SentryBuildOptions } from '@sentry/nextjs'

import robots from './app/robots'

// Under jsdom `@sentry/nextjs` resolves to its browser build, whose
// withSentryConfig returns the config untouched and applies no tunnel rewrite.
// So the options next.config.ts hands the wrapper are captured here instead.
const captured = vi.hoisted(() => ({ options: undefined as SentryBuildOptions | undefined }))

vi.mock('@sentry/nextjs', () => ({
  withSentryConfig: (config: unknown, options: SentryBuildOptions) => {
    captured.options = options
    return config
  },
}))

/**
 * robots.txt disallows the Sentry tunnel by a path it holds as its own
 * constant. This pins that path to the `tunnelRoute` the config sets, so a
 * change to either fails here.
 */
describe('the Sentry tunnel route', () => {
  it('is disallowed by every group robots.txt lets crawl', async () => {
    await import('./next.config')
    const tunnelRoute = captured.options?.tunnelRoute

    expect(typeof tunnelRoute).toBe('string')
    const { rules } = robots()
    const crawling = (Array.isArray(rules) ? rules : [rules]).filter(rule => rule.allow)
    expect(crawling.length).toBeGreaterThan(0)
    for (const rule of crawling) {
      expect(rule.disallow).toContain(tunnelRoute)
    }
  })
})
