import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { GoogleOAuthButton } from './google-oauth-button'
import { googleStartUrl } from './google-start-url'

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

  // A new identity is asked for its consent on the step after Google, so the
  // start carries none, whichever tab the button is on.
  it('starts with no sign-up consent and no account chooser', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GoogleOAuthButton returnTo="/shows" />)

    await user.click(screen.getByRole('button', { name: 'Continue with Google' }))

    expect([...startUrl().searchParams.keys()]).toEqual(['returnTo'])
  })

  it('is disabled when its surface says so', () => {
    renderWithProviders(<GoogleOAuthButton disabled />)

    expect(screen.getByRole('button', { name: 'Continue with Google' })).toBeDisabled()
  })
})

describe('googleStartUrl', () => {
  it('asks for the account chooser only when told to', () => {
    const chooser = new URL(googleStartUrl({ returnTo: '/', selectAccount: true }))
    expect(chooser.searchParams.get('prompt')).toBe('select_account')
    expect(chooser.searchParams.get('returnTo')).toBeNull()

    expect(new URL(googleStartUrl({ returnTo: '/' })).searchParams.get('prompt')).toBeNull()
  })
})
