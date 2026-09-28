import { describe, it, expect } from 'vitest'
import { sanitizeReturnTo } from '@/app/auth/auth-redirect-utils'
import {
  AUTH_INTENTS,
  AUTH_PATH,
  buildAuthHref,
  buildGatedAuthHref,
  buildReauthHref,
  currentLocationReturnTo,
  isReauthReason,
  hasGatedIntent,
  REAUTH_REASON_CREDENTIAL_MINT,
  REAUTH_REASON_OAUTH_LINK,
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

// A refused credential mint offers a link here. Two things have to hold or the
// remedy the copy names is a dead end: the reason has to be one the auth page
// recognizes, and returnTo has to bring the reader back to the control.
describe('buildReauthHref', () => {
  it('carries a reason the auth page treats as a re-authentication', () => {
    const href = buildReauthHref('/profile')
    expect(isReauthReason(new URL(href, 'https://x').searchParams.get('reason'))).toBe(
      true
    )
  })

  it('returns the reader to where they were refused', () => {
    const params = new URL(buildReauthHref('/profile?tab=settings'), 'https://x')
      .searchParams
    expect(sanitizeReturnTo(params.get('returnTo'))).toBe('/profile?tab=settings')
  })

  it('still carries the reason when there is no destination worth keeping', () => {
    const href = buildReauthHref('/')
    expect(new URL(href, 'https://x').searchParams.get('reason')).toBe(
      REAUTH_REASON_CREDENTIAL_MINT
    )
  })
})

describe('isReauthReason', () => {
  it('accepts both reasons the backend and the surfaces emit', () => {
    expect(isReauthReason(REAUTH_REASON_OAUTH_LINK)).toBe(true)
    expect(isReauthReason(REAUTH_REASON_CREDENTIAL_MINT)).toBe(true)
  })

  it('rejects an absent or unknown reason, which must not suppress the bounce', () => {
    expect(isReauthReason(null)).toBe(false)
    expect(isReauthReason('SOMETHING_ELSE')).toBe(false)
  })
})

// A gated control turned an anonymous viewer away. The page reads the intent
// back with hasGatedIntent, so every href built here has to carry one it
// accepts, alongside the returnTo the viewer came from.
describe('buildGatedAuthHref', () => {
  it('carries the returnTo and an accepted intent for every intent', () => {
    for (const intent of AUTH_INTENTS) {
      const params = new URL(
        buildGatedAuthHref('/shows?city=phoenix', intent),
        'https://x'
      ).searchParams
      expect(sanitizeReturnTo(params.get('returnTo'))).toBe('/shows?city=phoenix')
      expect(params.get('intent')).toBe(intent)
      expect(hasGatedIntent(params)).toBe(true)
    }
  })

  it('carries the intent alone when there is no destination worth keeping', () => {
    expect(buildGatedAuthHref('/', 'follow')).toBe('/auth?intent=follow')
  })
})

describe('hasGatedIntent', () => {
  it('accepts a listed intent', () => {
    expect(hasGatedIntent(new URLSearchParams('intent=save'))).toBe(true)
  })

  it('rejects an absent, unlisted, or near-miss value', () => {
    for (const query of [
      '',
      'returnTo=%2Fshows',
      'intent=',
      'intent=signup',
      'intent=SAVE',
      'intent=%20save',
      'intent=save,follow',
      'intent=https%3A%2F%2Fevil.example',
      'intent=%3Cscript%3E',
      'intent=constructor',
    ]) {
      expect(hasGatedIntent(new URLSearchParams(query))).toBe(false)
    }
  })

  // URLSearchParams.get returns the first value, so a later duplicate cannot
  // turn a rejected value into an accepted one.
  it('reads only the first of a repeated parameter', () => {
    expect(hasGatedIntent(new URLSearchParams('intent=bogus&intent=save'))).toBe(
      false
    )
  })
})
