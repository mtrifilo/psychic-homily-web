import { describe, it, expect, vi, afterEach } from 'vitest'
import { PHASE_DEVELOPMENT_SERVER, PHASE_PRODUCTION_BUILD } from 'next/constants'

// The global mock in test/setup omits `withSentryConfig`, which next.config
// calls at module scope. Identity keeps the config object as written.
vi.mock('@sentry/nextjs', async importOriginal => {
  const actual = await importOriginal<Record<string, unknown>>()
  return {
    ...actual,
    withSentryConfig: <T,>(config: T): T => config,
  }
})

import nextConfigForPhase from './next.config'

// The guard's own cases live in lib/build-api-origin-guard.test.ts. This file
// asserts only that the config Next loads actually runs it, with process.env.
describe('next.config build guard wiring', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('refuses a production build when process.env names no API origin', () => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', '')
    vi.stubEnv('VERCEL', '')

    expect(() => nextConfigForPhase(PHASE_PRODUCTION_BUILD)).toThrow(/NEXT_PUBLIC_API_URL/)
  })

  it('returns the config for a production build that names one', () => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', 'https://stage.api.psychichomily.com')
    vi.stubEnv('VERCEL', '')

    expect(nextConfigForPhase(PHASE_PRODUCTION_BUILD).cacheComponents).toBe(true)
  })

  it('returns the config for the dev server with no API origin', () => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', '')
    vi.stubEnv('VERCEL', '')

    expect(nextConfigForPhase(PHASE_DEVELOPMENT_SERVER).cacheComponents).toBe(true)
  })
})
