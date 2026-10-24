import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import userEvent from '@testing-library/user-event'
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

const TITLE = 'Add a passkey for faster sign-in'

const onLeave = vi.fn()

function renderCard() {
  return render(<PasskeyOfferCard onLeave={onLeave} />)
}

beforeEach(() => {
  profile = googleOnlyProfile()
  userId = '42'
  supportsWebAuthn = true
  registerOutcome = { ok: true }
  onLeave.mockReset()
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
    expect(onLeave).toHaveBeenCalledTimes(1)

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
    profile = googleOnlyProfile({ user: { ...googleOnlyProfile().user!, id: 43 } })
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
    const html = renderToString(<PasskeyOfferCard />)

    expect(html).not.toContain(TITLE)
  })

  it('is hidden for an account the profile does not qualify', () => {
    profile = googleOnlyProfile({ has_password: true })
    renderCard()

    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
  })

  it('is hidden while the profile still describes another account', () => {
    userId = '43'
    renderCard()

    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
  })

  it('retires at once when a passkey is added, before any profile refetch lands', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Add passkey' }))

    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
    expect(onLeave).toHaveBeenCalledTimes(1)
  })

  it('is hidden when the browser cannot make a passkey', () => {
    supportsWebAuthn = false
    renderCard()

    expect(screen.queryByRole('heading', { name: TITLE })).not.toBeInTheDocument()
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
  it('offers only on an explicit false and 0 for this Google account', () => {
    expect(profileOffersPasskey(googleOnlyProfile(), '42')).toBe(true)
    // An account with a passkey, or with a password (provider linked or not).
    expect(profileOffersPasskey(googleOnlyProfile({ passkey_count: 1 }), '42')).toBe(false)
    expect(profileOffersPasskey(googleOnlyProfile({ has_password: true }), '42')).toBe(false)
    expect(
      profileOffersPasskey(
        googleOnlyProfile({
          has_password: true,
          user: { ...googleOnlyProfile().user!, oauth_accounts: [] },
        }),
        '42'
      )
    ).toBe(false)
    // The copy names Google, so another provider does not qualify.
    expect(
      profileOffersPasskey(
        googleOnlyProfile({
          user: { ...googleOnlyProfile().user!, oauth_accounts: [{ provider: 'github' }] },
        }),
        '42'
      )
    ).toBe(false)
    // A profile still describing another account.
    expect(profileOffersPasskey(googleOnlyProfile(), '43')).toBe(false)
    expect(profileOffersPasskey(googleOnlyProfile({ has_password: undefined }), '42')).toBe(false)
    expect(profileOffersPasskey(googleOnlyProfile({ passkey_count: undefined }), '42')).toBe(false)
    expect(
      profileOffersPasskey(
        googleOnlyProfile({ user: { ...googleOnlyProfile().user!, oauth_accounts: undefined } }),
        '42'
      )
    ).toBe(false)
    expect(profileOffersPasskey(googleOnlyProfile({ success: false }), '42')).toBe(false)
    expect(profileOffersPasskey(undefined, '42')).toBe(false)
  })
})
