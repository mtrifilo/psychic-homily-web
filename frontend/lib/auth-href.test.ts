import { describe, it, expect } from 'vitest'
import { sanitizeReturnTo } from '@/app/auth/auth-redirect-utils'
import { AUTH_PATH, buildAuthHref } from './auth-href'

describe('buildAuthHref', () => {
  it('points at the auth route, which is the only route that renders the form', () => {
    // PSY-1870: `/login` does not exist and 404d for every reader who
    // followed a sign-in prompt.
    expect(AUTH_PATH).toBe('/auth')
    expect(buildAuthHref('/shows/example')).toBe(
      '/auth?returnTo=%2Fshows%2Fexample'
    )
  })

  it('encodes a fragment so it survives as part of returnTo', () => {
    expect(buildAuthHref('/shows/example#comments')).toBe(
      '/auth?returnTo=%2Fshows%2Fexample%23comments'
    )
  })

  it('encodes a query string instead of leaking it into the auth page params', () => {
    // Unencoded, the `&` would read as a second param on /auth itself.
    expect(buildAuthHref('/users/alice?tab=bio&sort=new')).toBe(
      '/auth?returnTo=%2Fusers%2Falice%3Ftab%3Dbio%26sort%3Dnew'
    )
  })

  it('round-trips through sanitizeReturnTo, the inverse the auth page applies', () => {
    const destinations = [
      '/shows/example',
      '/shows/example#comments',
      '/users/alice?tab=bio',
    ]

    for (const destination of destinations) {
      const href = buildAuthHref(destination)
      const returnTo = new URL(href, 'https://psychichomily.com').searchParams.get(
        'returnTo'
      )
      expect(sanitizeReturnTo(returnTo)).toBe(destination)
    }
  })
})
