import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { renderWithProviders } from '@/test/utils'
import { server } from '@/test/mocks/server'
import { TEST_API_BASE } from '@/test/mocks/handlers'
import { GoogleOAuthButton } from './google-oauth-button'
import { GOOGLE_SIGNUP_START_KEY } from './google-signup-start'

vi.mock('@/lib/api-base', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api-base')>()),
  OAUTH_BACKEND_URL: 'https://oauth-backend.test',
}))

// The button leaves the page by assigning window.location.href; the setter
// records the start URL instead of navigating.
const navigatedTo = vi.fn<(href: string) => void>()
const originalLocation = window.location

function startUrl(): URL {
  expect(navigatedTo).toHaveBeenCalledTimes(1)
  return new URL(navigatedTo.mock.calls[0][0])
}

describe('GoogleOAuthButton', () => {
  beforeEach(() => {
    navigatedTo.mockReset()
    sessionStorage.clear()
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation },
      writable: true,
      configurable: true,
    })
    Object.defineProperty(window.location, 'href', {
      set: navigatedTo,
      configurable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
  })

  it.each([
    ['a destination', '/profile?tab=settings', '/profile?tab=settings'],
    ['no destination', undefined, null],
    ['the site root', '/', null],
  ])(
    'hands the sign-in start %s',
    async (_label, returnTo: string | undefined, expected: string | null) => {
      const user = userEvent.setup()
      renderWithProviders(<GoogleOAuthButton returnTo={returnTo} />)

      await user.click(
        screen.getByRole('button', { name: 'Continue with Google' })
      )

      const url = startUrl()
      expect(url.origin + url.pathname).toBe(
        'https://oauth-backend.test/auth/login/google'
      )
      expect(url.searchParams.get('returnTo')).toBe(expected)
    }
  )

  it('carries the destination beside the sign-up consent', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <GoogleOAuthButton
        variant="signup"
        termsAccepted
        ageConfirmed
        returnTo="/shows"
      />
    )

    await user.click(screen.getByRole('button', { name: 'Sign up with Google' }))

    await waitFor(() => expect(navigatedTo).toHaveBeenCalled())
    const params = startUrl().searchParams
    expect(params.get('signup_intent')).toBe('1')
    expect(params.get('terms_accepted')).toBe('true')
    expect(params.get('age_confirmed')).toBe('true')
    expect(params.get('min_age_attested')).toBe('16')
    expect(params.get('returnTo')).toBe('/shows')
  })

  describe('consent versions', () => {
    const PUBLISHED = { terms_version: '2031-03-01', privacy_version: '2031-03-02' }

    function renderSignup(onConsentVersionsUnavailable = vi.fn()) {
      renderWithProviders(
        <GoogleOAuthButton
          variant="signup"
          termsAccepted
          ageConfirmed
          onConsentVersionsUnavailable={onConsentVersionsUnavailable}
        />
      )
      return onConsentVersionsUnavailable
    }

    it('starts the sign-up with the versions the route publishes and records the start', async () => {
      server.use(
        http.get(`${TEST_API_BASE}/legal/versions`, () =>
          HttpResponse.json({ ...PUBLISHED, min_signup_age: 16, grace_until: null })
        )
      )
      const user = userEvent.setup()
      renderSignup()

      await user.click(screen.getByRole('button', { name: 'Sign up with Google' }))

      await waitFor(() => expect(navigatedTo).toHaveBeenCalled())
      const params = startUrl().searchParams
      expect(params.get('terms_version')).toBe('2031-03-01')
      expect(params.get('privacy_version')).toBe('2031-03-02')
      expect(sessionStorage.getItem(GOOGLE_SIGNUP_START_KEY)).not.toBeNull()
    })

    it('does not leave the page, and reports it, when the versions cannot be read', async () => {
      server.use(
        http.get(`${TEST_API_BASE}/legal/versions`, () =>
          HttpResponse.json({ detail: 'unavailable' }, { status: 503 })
        )
      )
      const user = userEvent.setup()
      const onConsentVersionsUnavailable = renderSignup()

      await user.click(screen.getByRole('button', { name: 'Sign up with Google' }))

      await waitFor(() => expect(onConsentVersionsUnavailable).toHaveBeenCalled())
      expect(navigatedTo).not.toHaveBeenCalled()
      expect(sessionStorage.getItem(GOOGLE_SIGNUP_START_KEY)).toBeNull()
      expect(screen.getByRole('button', { name: 'Sign up with Google' })).toBeEnabled()
    })

    it('drops a recorded sign-up start when a sign-in starts', async () => {
      sessionStorage.setItem(GOOGLE_SIGNUP_START_KEY, String(Date.now()))
      const user = userEvent.setup()
      renderWithProviders(<GoogleOAuthButton />)

      await user.click(screen.getByRole('button', { name: 'Continue with Google' }))

      expect(startUrl().searchParams.get('signup_intent')).toBeNull()
      expect(sessionStorage.getItem(GOOGLE_SIGNUP_START_KEY)).toBeNull()
    })
  })

  it('does not leave the page while sign-up consent is missing', async () => {
    const user = userEvent.setup()
    const onMissingTermsAcceptance = vi.fn()
    renderWithProviders(
      <GoogleOAuthButton
        variant="signup"
        returnTo="/shows"
        onMissingTermsAcceptance={onMissingTermsAcceptance}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Sign up with Google' }))

    expect(onMissingTermsAcceptance).toHaveBeenCalled()
    expect(navigatedTo).not.toHaveBeenCalled()
  })
})
