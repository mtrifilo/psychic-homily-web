import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { PHASE_DEVELOPMENT_SERVER, PHASE_PRODUCTION_BUILD } from 'next/constants'
import nextConfigForPhase from './next.config'

// The guard's own cases live in lib/build-api-origin-guard.test.ts. This file
// asserts only that the config Next loads runs it with process.env and the
// phase Next passes in.
describe('next.config build guard wiring', () => {
  beforeEach(() => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', '')
    vi.stubEnv('VERCEL', '')
  })

  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('refuses a production build when process.env names no API origin', () => {
    expect(() => nextConfigForPhase(PHASE_PRODUCTION_BUILD)).toThrow(/NEXT_PUBLIC_API_URL/)
  })

  it('returns the config for a production build that names one', () => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', 'https://stage.api.psychichomily.com')

    expect(nextConfigForPhase(PHASE_PRODUCTION_BUILD).cacheComponents).toBe(true)
  })

  it('returns the config for the dev server with no API origin', () => {
    expect(nextConfigForPhase(PHASE_DEVELOPMENT_SERVER).cacheComponents).toBe(true)
  })
})
