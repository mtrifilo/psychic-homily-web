import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  PHASE_DEVELOPMENT_SERVER,
  PHASE_PRODUCTION_BUILD,
  PHASE_PRODUCTION_SERVER,
} from 'next/constants'
import nextConfigForPhase, { assertBuildApiOriginConfigured } from './next.config'

// The guard takes its env as an argument, so vitest's pinned
// NEXT_PUBLIC_API_URL cannot leak into the unset cases.
describe('assertBuildApiOriginConfigured', () => {
  it('refuses a production build with no API URL outside Vercel', () => {
    expect(() => assertBuildApiOriginConfigured(PHASE_PRODUCTION_BUILD, {})).toThrow(
      /NEXT_PUBLIC_API_URL.*https:\/\/stage\.api\.psychichomily\.com.*VERCEL/,
    )
  })

  it('treats an empty API URL as unset', () => {
    expect(() =>
      assertBuildApiOriginConfigured(PHASE_PRODUCTION_BUILD, { NEXT_PUBLIC_API_URL: '' }),
    ).toThrow(/NEXT_PUBLIC_API_URL/)
  })

  it('does not count BACKEND_URL as naming the data origin', () => {
    expect(() =>
      assertBuildApiOriginConfigured(PHASE_PRODUCTION_BUILD, {
        BACKEND_URL: 'https://stage.api.psychichomily.com',
      }),
    ).toThrow(/NEXT_PUBLIC_API_URL/)
  })

  it('passes a production build that sets the API URL', () => {
    expect(() =>
      assertBuildApiOriginConfigured(PHASE_PRODUCTION_BUILD, {
        NEXT_PUBLIC_API_URL: 'https://stage.api.psychichomily.com',
      }),
    ).not.toThrow()
  })

  it('passes a Vercel build even with no API URL', () => {
    expect(() =>
      assertBuildApiOriginConfigured(PHASE_PRODUCTION_BUILD, { VERCEL: '1' }),
    ).not.toThrow()
  })

  it.each([PHASE_DEVELOPMENT_SERVER, PHASE_PRODUCTION_SERVER])(
    'does not check the %s phase',
    phase => {
      expect(() => assertBuildApiOriginConfigured(phase, {})).not.toThrow()
    },
  )
})

// The config Next loads must run the guard with process.env and the phase Next
// passes in.
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
