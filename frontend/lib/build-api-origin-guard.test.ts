import { describe, it, expect } from 'vitest'
import {
  PHASE_DEVELOPMENT_SERVER,
  PHASE_PRODUCTION_BUILD,
  PHASE_PRODUCTION_SERVER,
} from 'next/constants'
import { assertBuildApiOriginConfigured } from './build-api-origin-guard'

// The env is passed explicitly rather than read from process.env, so vitest's
// pinned NEXT_PUBLIC_API_URL cannot leak into the unset cases.
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
