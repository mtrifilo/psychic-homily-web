import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { GoogleOAuthButton } from './google-oauth-button'

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

  it('carries the destination to the sign-in start', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GoogleOAuthButton returnTo="/profile?tab=settings" />)

    await user.click(screen.getByRole('button', { name: 'Continue with Google' }))

    const url = startUrl()
    expect(url.origin + url.pathname).toBe(
      'https://oauth-backend.test/auth/login/google'
    )
    expect(url.searchParams.get('returnTo')).toBe('/profile?tab=settings')
  })

  it('names no destination when the reader came from nowhere in particular', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GoogleOAuthButton />)

    await user.click(screen.getByRole('button', { name: 'Continue with Google' }))

    expect(startUrl().searchParams.has('returnTo')).toBe(false)
  })

  it('names no destination for the site root', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GoogleOAuthButton returnTo="/" />)

    await user.click(screen.getByRole('button', { name: 'Continue with Google' }))

    expect(startUrl().searchParams.has('returnTo')).toBe(false)
  })

  it('carries the destination beside the sign-up consent', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <GoogleOAuthButton
        variant="signup"
        termsAccepted
        termsVersion="2026-01-31"
        privacyVersion="2026-02-15"
        ageConfirmed
        minAge={16}
        returnTo="/shows"
      />
    )

    await user.click(screen.getByRole('button', { name: 'Sign up with Google' }))

    const params = startUrl().searchParams
    expect(params.get('signup_intent')).toBe('1')
    expect(params.get('terms_accepted')).toBe('true')
    expect(params.get('age_confirmed')).toBe('true')
    expect(params.get('returnTo')).toBe('/shows')
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
