import { describe, it, expect } from 'vitest'
import { sanitizeReturnTo } from '@/app/auth/auth-redirect-utils'
import {
  AUTH_PATH,
  buildAuthHref,
  currentLocationReturnTo,
} from './auth-href'

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

describe('currentLocationReturnTo', () => {
  it('carries the query string, which is where the hand-rolled copies drifted', () => {
    // Four of the nine copies this replaced sent the bare pathname, so a
    // reader who clicked from a filtered list came back to the unfiltered page.
    window.history.replaceState({}, '', '/shows?city=phoenix&when=weekend')
    expect(currentLocationReturnTo('/shows')).toBe(
      '/shows?city=phoenix&when=weekend'
    )
  })

  it('is the bare pathname when the location carries no query', () => {
    window.history.replaceState({}, '', '/artists/calexico')
    expect(currentLocationReturnTo('/artists/calexico')).toBe(
      '/artists/calexico'
    )
  })

  it('round-trips through the auth page inverse', () => {
    window.history.replaceState({}, '', '/users/alice?tab=bio')
    const href = buildAuthHref(currentLocationReturnTo('/users/alice'))
    const returnTo = new URL(href, 'https://psychichomily.com').searchParams.get(
      'returnTo'
    )
    expect(sanitizeReturnTo(returnTo)).toBe('/users/alice?tab=bio')
  })
})
