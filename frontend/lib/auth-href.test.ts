import { describe, it, expect } from 'vitest'
import { sanitizeReturnTo } from '@/app/auth/auth-redirect-utils'
import {
  ACCOUNT_INACTIVE_REASON,
  AUTH_ERROR_CODE_PARAM,
  AUTH_ERROR_PARAM,
  AUTH_INTENTS,
  AUTH_PATH,
  buildAuthHref,
  buildGatedAuthHref,
  buildReauthHref,
  currentLocationReturnTo,
  isReauthReason,
  hasGatedIntent,
  isAccountRecoverableRefusal,
  needsSignupConsent,
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

  it('drops an OAuth link error so a later sign-in does not replay it', () => {
    window.history.replaceState(
      {},
      '',
      '/profile?tab=settings&oauth_link_error=ACCOUNT_INACTIVE'
    )
    expect(currentLocationReturnTo('/profile')).toBe('/profile?tab=settings')
  })

  it('drops an OAuth link success the same way', () => {
    window.history.replaceState({}, '', '/profile?oauth_link=connected&tab=settings')
    expect(currentLocationReturnTo('/profile')).toBe('/profile?tab=settings')
  })

  it('is the bare pathname when the link result was the whole query', () => {
    window.history.replaceState(
      {},
      '',
      '/profile?oauth_link_error=OAUTH_LINK_REAUTH_REQUIRED&oauth_link=connected'
    )
    expect(currentLocationReturnTo('/profile')).toBe('/profile')
  })

  it('recognizes a link parameter by its decoded name, as Settings reads it', () => {
    window.history.replaceState({}, '', '/profile?tab=settings&oauth%5Flink%5Ferror=X')
    expect(currentLocationReturnTo('/profile')).toBe('/profile?tab=settings')
  })

  // A URL parser names the pair "?oauth_link", which Settings does not read.
  it('keeps a pair whose name begins with a question mark', () => {
    window.history.replaceState({}, '', '/profile??oauth_link=connected&tab=settings')
    expect(currentLocationReturnTo('/profile')).toBe(
      '/profile??oauth_link=connected&tab=settings'
    )
  })

  it('leaves every other parameter exactly as the location spells it', () => {
    window.history.replaceState(
      {},
      '',
      '/shows?q=a%20b&oauth_link=connected&city=phoenix&oauth_linked=1'
    )
    expect(currentLocationReturnTo('/shows')).toBe(
      '/shows?q=a%20b&city=phoenix&oauth_linked=1'
    )
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

  // Signing in again cannot reopen a closed account, so this reason must not
  // turn the page into the re-authentication form.
  it('does not count a closed account as a re-authentication', () => {
    expect(isReauthReason(ACCOUNT_INACTIVE_REASON)).toBe(false)
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

describe('isAccountRecoverableRefusal', () => {
  it('accepts the recoverable refusal code', () => {
    expect(
      isAccountRecoverableRefusal(
        new URLSearchParams('error=x&error_code=ACCOUNT_RECOVERABLE')
      )
    ).toBe(true)
  })

  it('rejects an absent, different, or near-miss code', () => {
    for (const query of [
      '',
      'error=This+account+was+deleted+recently',
      'error_code=',
      'error_code=ACCOUNT_INACTIVE',
      'error_code=account_recoverable',
      'error_code=%20ACCOUNT_RECOVERABLE',
      'error_code=ACCOUNT_RECOVERABLE_X',
    ]) {
      expect(isAccountRecoverableRefusal(new URLSearchParams(query))).toBe(false)
    }
  })
})

describe('needsSignupConsent', () => {
  // The backend's authPageErrorParam and authPageErrorCodeParam spell the
  // same names.
  it('reads the error and error_code parameters', () => {
    expect(AUTH_ERROR_PARAM).toBe('error')
    expect(AUTH_ERROR_CODE_PARAM).toBe('error_code')
  })

  it('accepts the terms refusal code', () => {
    expect(
      needsSignupConsent(
        new URLSearchParams('error=x&error_code=TERMS_ACCEPTANCE_REQUIRED')
      )
    ).toBe(true)
  })

  it('rejects an absent, different, or near-miss code', () => {
    for (const query of [
      '',
      'error=Please+accept+the+Terms+of+Service',
      'error_code=',
      'error_code=USER_EXISTS',
      'error_code=terms_acceptance_required',
      'error_code=%20TERMS_ACCEPTANCE_REQUIRED',
      'error_code=TERMS_ACCEPTANCE_REQUIRED_X',
      'error_code=TERMS%2541CCEPTANCE_REQUIRED',
    ]) {
      expect(needsSignupConsent(new URLSearchParams(query))).toBe(false)
    }
  })

  it('reads only the first of a repeated parameter', () => {
    expect(
      needsSignupConsent(
        new URLSearchParams(
          'error_code=USER_EXISTS&error_code=TERMS_ACCEPTANCE_REQUIRED'
        )
      )
    ).toBe(false)
  })
})
