import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { AuthError, AuthErrorCode, REAUTH_REQUIRED_MESSAGE } from '@/lib/errors'
import { buildReauthHref } from '@/lib/auth-href'
import type { UserProfile } from '@/features/auth/hooks/useAuth'
import { PasskeyOfferCard, profileOffersPasskey } from './PasskeyOfferCard'

let profile: UserProfile | undefined
vi.mock('@/features/auth/hooks/useAuth', () => ({
  useProfile: () => ({ data: profile }),
}))

let userId = '42'
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ user: { id: userId }, authStatus: 'authenticated' }),
}))

let supportsWebAuthn = true
vi.mock('@/features/auth/hooks/useWebAuthnSupport', () => ({
  useWebAuthnSupport: () => supportsWebAuthn,
}))

const mockRefetchProfile = vi.fn()
vi.mock('@/lib/queryClient', () => ({
  refetchSignedInProfile: (...args: unknown[]) => mockRefetchProfile(...args),
}))

vi.mock('next/navigation', () => ({
  usePathname: () => '/',
}))

// The ceremony is PasskeyRegisterButton's own and is tested there. The stand-in
// keeps its trigger name and lets each test pick the outcome.
let registerOutcome: { ok: true } | { ok: false; error: unknown } = { ok: true }
vi.mock('@/features/auth/components/PasskeyRegisterButton', () => ({
  PasskeyRegisterButton: ({
    onSuccess,
    onError,
  }: {
    onSuccess?: () => void
    onError?: (error: unknown) => void
  }) => (
    <button
      type="button"
      onClick={() =>
        registerOutcome.ok ? onSuccess?.() : onError?.(registerOutcome.error)
      }
    >
      Add passkey
    </button>
  ),
}))

function googleOnlyProfile(overrides: Partial<UserProfile> = {}): UserProfile {
  return {
    success: true,
    message: 'Profile retrieved',
    user: {
      id: 42,
      email: 'new@example.com',
      created_at: '2026-10-24T00:00:00Z',
      updated_at: '2026-10-24T00:00:00Z',
      oauth_accounts: [{ provider: 'google' }],
    },
    has_password: false,
    passkey_count: 0,
    ...overrides,
  }
}

function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
  )
}

const TITLE = 'Add a passkey for faster sign-in'

function renderCard() {
  return render(<PasskeyOfferCard />, { wrapper })
}

beforeEach(() => {
  profile = googleOnlyProfile()
  userId = '42'
  supportsWebAuthn = true
  registerOutcome = { ok: true }
  mockRefetchProfile.mockClear()
  window.localStorage.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('PasskeyOfferCard', () => {
  it('offers a passkey to a provider-created account with no password and no passkey', () => {
    renderCard()

    expect(screen.getByRole('heading', { name: TITLE })).toBeInTheDocument()
    expect(
      screen.getByText(
        'Next time, sign in with your fingerprint, face or screen lock, no trip to Google.'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add passkey' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Not now' })).toBeInTheDocument()
  })

  it('is not a modal: no dialog, nothing taken from the page', () => {
    renderCard()

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByRole('region', { name: TITLE })).toBeInTheDocument()
  })

  it('stays dismissed after Not now, across a reload', async () => {
    const user = userEvent.setup()
    const { unmount } = renderCard()

    await user.click(screen.getByRole('button', { name: 'Not now' }))
    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()

    unmount()
    renderCard()
    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
  })

  it('dismisses for this account only', async () => {
    const user = userEvent.setup()
    const { unmount } = renderCard()
    await user.click(screen.getByRole('button', { name: 'Not now' }))
    unmount()

    userId = '43'
    renderCard()
    expect(screen.getByRole('heading', { name: TITLE })).toBeInTheDocument()
  })

  it('hides in the page when the dismissal cannot be stored, and may offer again on the next visit', async () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError')
    })
    const user = userEvent.setup()
    const { unmount } = renderCard()

    await user.click(screen.getByRole('button', { name: 'Not now' }))
    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()

    unmount()
    renderCard()
    expect(screen.getByRole('heading', { name: TITLE })).toBeInTheDocument()
  })

  it('is absent from the server HTML, so a dismissed card never flashes', () => {
    const html = renderToString(wrapper({ children: <PasskeyOfferCard /> }))

    expect(html).not.toContain(TITLE)
  })

  it('is hidden once the account has a passkey', () => {
    profile = googleOnlyProfile({ passkey_count: 1 })
    renderCard()

    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
  })

  it('is hidden for a password account, with or without a linked provider', () => {
    profile = googleOnlyProfile({ has_password: true })
    const { unmount } = renderCard()
    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
    unmount()

    profile = googleOnlyProfile({
      has_password: true,
      user: { ...googleOnlyProfile().user!, oauth_accounts: [] },
    })
    renderCard()
    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
  })

  it('is hidden when the browser cannot make a passkey', () => {
    supportsWebAuthn = false
    renderCard()

    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
  })

  it('refetches the profile after a passkey is added, which is what retires the card', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Add passkey' }))

    expect(mockRefetchProfile).toHaveBeenCalledTimes(1)
  })

  it('sends a viewer whose sign-in is too old to sign in again, as Settings does', async () => {
    registerOutcome = {
      ok: false,
      error: new AuthError('Reauthentication required', AuthErrorCode.REAUTH_REQUIRED),
    }
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Add passkey' }))

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(REAUTH_REQUIRED_MESSAGE)
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
      'href',
      buildReauthHref('/')
    )
    expect(screen.getByRole('heading', { name: TITLE })).toBeInTheDocument()
  })

  it('shows an ordinary failure in the card and keeps the offer', async () => {
    registerOutcome = { ok: false, error: new Error('Failed to register passkey') }
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Add passkey' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Failed to register passkey')
    expect(screen.queryByRole('link', { name: 'Sign in again' })).not.toBeInTheDocument()
  })
})

describe('profileOffersPasskey', () => {
  it('offers only on an explicit false and 0 for a provider-created account', () => {
    expect(profileOffersPasskey(googleOnlyProfile())).toBe(true)
    expect(profileOffersPasskey(googleOnlyProfile({ has_password: undefined }))).toBe(false)
    expect(profileOffersPasskey(googleOnlyProfile({ passkey_count: undefined }))).toBe(false)
    expect(
      profileOffersPasskey(
        googleOnlyProfile({ user: { ...googleOnlyProfile().user!, oauth_accounts: undefined } })
      )
    ).toBe(false)
    expect(profileOffersPasskey(googleOnlyProfile({ success: false }))).toBe(false)
    expect(profileOffersPasskey(undefined)).toBe(false)
  })
})
